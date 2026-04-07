package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	auth "github.com/iden3/go-iden3-auth/v2"
	"github.com/iden3/go-iden3-auth/v2/loaders"
	"github.com/iden3/go-iden3-auth/v2/pubsignals"
	"github.com/iden3/go-iden3-auth/v2/state"
	core "github.com/iden3/go-iden3-core/v2"
	"github.com/iden3/iden3comm/v2/packers"
	log "github.com/sirupsen/logrus"

	"github.com/0xPolygonID/verifier-backend/internal/api"
	"github.com/0xPolygonID/verifier-backend/internal/config"
	"github.com/0xPolygonID/verifier-backend/internal/errors"
	"github.com/0xPolygonID/verifier-backend/internal/loader"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.WithField("error", err).Error("cannot load config")
		return
	}

	mux := chi.NewRouter()

	mux.Use(
		chiMiddleware.RequestID,
		chiMiddleware.Recoverer,
		cors.Handler(cors.Options{AllowedOrigins: []string{"*"}}),
		chiMiddleware.NoCache,
	)

	keysLoader := &loaders.FSKeyLoader{Dir: cfg.KeyDIR}
	w3cLoader := loader.NewW3CDocumentLoader(nil, cfg.IPFSURL)

	resolvers, senderDIDs, err := parseResolverSettings(ctx, cfg.ResolverSettings)
	if err != nil {
		log.WithField("error", err).Error("cannot parse resolver settings")
		return
	}

	verifier, err := auth.NewVerifier(
		keysLoader,
		resolvers,
		auth.WithDocumentLoader(w3cLoader),
	)
	if err != nil {
		log.WithField("error", err).Error("failed to create verifier")
		return
	}

	verifier.SetPacker(&packers.PlainMessagePacker{})

	apiServer := api.New(*cfg, verifier, senderDIDs)
	api.HandlerFromMux(
		api.NewStrictHandlerWithOptions(
			apiServer,
			nil,
			api.StrictHTTPServerOptions{
				RequestErrorHandlerFunc: errors.RequestErrorHandlerFunc,
			},
		),
		mux,
	)
	api.RegisterStatic(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.ApiPort),
		Handler: mux,
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.WithField("port", cfg.ApiPort).Info("server started")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithField("error", err).Error("starting http server")
		}
	}()

	<-quit
	log.Info("Shutting down")
}

func parseResolverSettings(
	ctx context.Context,
	rs config.ResolverSettings,
) (map[string]pubsignals.StateResolver, map[string]string, error) {
	var (
		resolvers     = make(map[string]pubsignals.StateResolver)
		verifiersDIDs = make(map[string]string)
	)

	for chainName, chainSettings := range rs {
		for networkName, networkSettings := range chainSettings {
			if err := registerCustomDIDMethod(ctx, chainName, networkName, networkSettings); err != nil {
				log.WithFields(log.Fields{
					"chain":   chainName,
					"network": networkName,
					"error":   err,
				}).Error("cannot register custom DID method")
				return nil, nil, err
			}

			prefix := fmt.Sprintf("%s:%s", chainName, networkName)
			resolver := state.NewETHResolver(
				networkSettings.NetworkURL,
				networkSettings.ContractAddress,
			)
			resolvers[prefix] = resolver
			verifiersDIDs[networkSettings.ChainID] = networkSettings.DID
		}
	}

	return resolvers, verifiersDIDs, nil
}

func registerCustomDIDMethod(
	_ context.Context,
	blockchain string,
	network string,
	resolverAttrs config.ResolverSettingsAttrs,
) error {
	chainID, err := strconv.Atoi(resolverAttrs.ChainID)
	if err != nil {
		return fmt.Errorf("cannot convert chainID to int: %w", err)
	}

	params := core.DIDMethodNetworkParams{
		Method:      core.DIDMethod(resolverAttrs.Method),
		Blockchain:  core.Blockchain(blockchain),
		Network:     core.NetworkID(network),
		NetworkFlag: resolverAttrs.NetworkFlag,
	}

	err = core.RegisterDIDMethodNetwork(params, core.WithChainID(chainID))
	if err != nil {
		// Avoid failing startup if already registered by another path/init.
		if err.Error() != "DID method network already registered" {
			return err
		}
	}

	return nil
}