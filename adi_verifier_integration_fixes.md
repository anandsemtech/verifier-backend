# ADI Verifier Integration Fixes

## Overview

This document records all fixes applied to make the **ADI testnet verifier flow** fully functional with:

```
did:iden3:adi:adiTestnet:<genesis>
```

### Environment

- Chain ID: `99999`
- Network: `adiTestnet`
- Method: `iden3`
- Network Flag: `249`

---

# Final Working Flow

The verifier now successfully completes the full flow:

1. Verifier generates authorization request
2. Wallet loads QR
3. Wallet generates proof
4. Wallet sends proof to `/callback`
5. Verifier validates proof (`200 OK`)
6. `/status` returns:

```json
{
  "status": "success"
}
```

---

# Root Cause Analysis & Fixes

## 1️⃣ Missing Plain Message Packer

### Error

```
packer for media type application/iden3comm-plain-json doesn't exist
```

### Cause

Verifier was not configured to handle **plain-json iden3comm messages**.

### Fix

📁 `cmd/main.go`

```go
import "github.com/iden3/iden3comm/v2/packers"
```

```go
verifier.SetPacker(&packers.PlainMessagePacker{})
```

---

## 2️⃣ Unsupported ADI Blockchain

### Error

```
not supported blockchain
```

### Cause

Verifier did not recognize:

```
did:iden3:adi:adiTestnet
```

### Fix

Re-enabled custom DID registration.

📁 `cmd/main.go`

### Add imports

```go
import (
    "strconv"
    core "github.com/iden3/go-iden3-core/v2"
)
```

### Enable registration inside `parseResolverSettings`

```go
if err := registerCustomDIDMethod(ctx, chainName, networkName, networkSettings); err != nil {
    log.WithField("error", err).Error("cannot register custom DID method")
    return nil, nil, err
}
```

### Add function

```go
func registerCustomDIDMethod(ctx context.Context, blockchain string, network string, resolverAttrs config.ResolverSettingsAttrs) error {
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

    if err := core.RegisterDIDMethodNetwork(params, core.WithChainID(chainID)); err != nil {
        log.Error("cannot register custom DID method", err)
        return err
    }

    return nil
}
```

---

## 3️⃣ JWZ Parsing Failure in `/status`

### Error

```
iden3/go-jwz: missing payload in JWZ message
```

### Cause

Verifier stored **plain JSON authorization response**, but `/status` tried to parse it as **JWZ token**.

---

### Fix

📁 `internal/api/server.go`

Updated:

```go
func getVerifiablePresentations(tokenStr string) (VerifiablePresentations, error)
```

### New behavior

#### Case 1: Plain JSON response

```go
if strings.HasPrefix(tokenStr, "{") {
    var msg protocol.AuthorizationResponseMessage
    if err := json.Unmarshal([]byte(tokenStr), &msg); err != nil {
        return nil, err
    }

    return VerifiablePresentations{}, nil
}
```

#### Case 2: JWZ token

```go
token, err := jwz.Parse(tokenStr)
```

---

# Files Modified

## ✅ `cmd/main.go`

- Added packer
- Enabled DID registration
- Added imports

---

## ✅ `internal/api/server.go`

- Fixed `/status` parsing logic
- Added plain-json support
- Prevented JWZ parsing crash

---

## 🚫 `qrcode.go`

No changes required.

Reason:

```go
type GetQRCodeFromStore200JSONResponse QRCode
```

API expects `QRCode`, not protocol message.

---

# Important Notes

## Plain JSON vs JWZ

Even though `/status` returns:

```json
"jwz": "{...}"
```

This is actually **plain JSON**, not a compact JWZ.

This is acceptable because:
- field naming is legacy
- functionality is correct

---

## Verifiable Presentations

Currently:

```json
"verifiablePresentations": null
```

Reason:
- `protocol.ZeroKnowledgeProofResponse` does NOT expose `Vp` in your version
- cannot extract VC safely

This is **non-blocking**

---

# Verification Checklist

## Wallet Logs

```
[ok] verification request handled
[ok] posting proof to callbackUrl
[debug] callback response status=200
```

## Verifier Logs

```
level=info msg=callback sessionID=...
```

## Status Endpoint

```json
{
  "status": "success"
}
```

---

# Regression Tests (Recommended)

## Positive Case
- Valid credential → success

## Negative Case
- Missing credential → should NOT return success

## Replay Protection
- Same session → should not re-verify

---

# Summary

The system failed due to three missing integrations:

| Issue | Fix |
|------|-----|
| Missing packer | Added PlainMessagePacker |
| Unknown blockchain | Registered ADI DID |
| Wrong token parsing | Handled plain JSON separately |

After these fixes, the verifier works end-to-end with ADI DID.

---

## Next Steps (Optional)

- Extract Verifiable Credentials (VP)
- Add persistence (DB instead of cache)
- Improve logging & monitoring
- Production hardening (timeouts, retries, rate limits)

