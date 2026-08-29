# Security Fix: Session Cookie Protection

## Issue
Session cookies (`d3_session`) were being set without the `Secure` attribute, allowing them to be transmitted over plaintext HTTP connections. Combined with an nginx configuration that accepted HTTP traffic on port 80, this created a session hijacking vulnerability where network-positioned attackers could intercept and replay session tokens.

## Root Cause
1. **auth.go**: Three locations set `d3_session` cookies without `Secure`:
   - `callback()` function (OAuth flow) - line 135
   - `Bootstrap()` function (anonymous user creation) - line 175
   - `rewriteCookie()` function explicitly stripped `Secure` from upstream cookies - line 256

2. **devauth.go**: Development auth also set cookies without `Secure` (line 61)

3. **nginx configuration**: Accepted HTTP on port 80 and proxied to the application instead of redirecting to HTTPS

## Solution

### Application Changes (auth.go)
1. Added `isSecureRequest()` helper function that checks the `X-Forwarded-Proto` header set by nginx
2. Updated `callback()` to set `Secure: isSecureRequest(r)` on the session cookie
3. Updated `Bootstrap()` to set `Secure: isSecureRequest(r)` on the session cookie
4. Updated `rewriteCookie()` to preserve the `Secure` attribute when the request arrived over HTTPS
5. Updated `proxy()` to pass the secure flag to `rewriteCookie()`

### Application Changes (devauth.go)
- Updated `mint()` function comment to document that dev auth is local-only and doesn't set Secure
- Maintained consistency with production auth behavior

### Infrastructure Changes (nginx)
1. Split HTTP and HTTPS server blocks
2. HTTP block (port 80) now redirects all traffic to HTTPS except ACME challenges
3. HTTPS block retains all application proxying logic
4. Preserved `X-Forwarded-Proto` header forwarding (already present on line 62)

## Behavior

### Production (with HTTPS)
- nginx receives HTTPS request
- nginx sets `X-Forwarded-Proto: https` header
- Application detects secure request via `isSecureRequest()`
- Session cookies are set with `Secure` attribute
- Cookies are only sent over HTTPS connections
- HTTP requests are redirected to HTTPS by nginx

### Development (localhost)
- No nginx proxy, direct connection to Go application
- No `X-Forwarded-Proto` header present
- `isSecureRequest()` returns false
- Session cookies are set without `Secure` attribute
- Local development over HTTP continues to work

### Dev Auth Mode
- Uses same cookie-setting logic as production auth
- Always runs on localhost without nginx
- Cookies never have `Secure` attribute
- This is acceptable because `-dev-auth` must never be exposed publicly

## Deployment Notes

### Initial Setup (No TLS Yet)
If deploying for the first time without TLS certificates:
1. Comment out the HTTP redirect server block in nginx config (lines 7-25)
2. Deploy and run certbot to obtain certificates
3. Uncomment the HTTP redirect block
4. Reload nginx

### Existing Deployment (TLS Already Configured)
1. Deploy the updated nginx configuration
2. Reload nginx: `sudo systemctl reload nginx`
3. Deploy the updated Go application
4. Restart the service: `sudo systemctl restart deez-win`

## Testing

### Verify Secure Cookies (Production)
```bash
# Make an HTTPS request and check the Set-Cookie header
curl -v https://deez.win/ 2>&1 | grep -i "set-cookie"
# Should see: Set-Cookie: d3_session=...; Secure; HttpOnly; ...
```

### Verify HTTP Redirect
```bash
# Make an HTTP request
curl -v http://deez.win/ 2>&1 | grep -i "location"
# Should see: Location: https://deez.win/
```

### Verify Local Development Still Works
```bash
# Start the app locally
./run.sh
# Visit http://localhost:8080
# Should be able to play without issues
```

## Security Impact

**Before**: Session tokens could be intercepted over HTTP, allowing session hijacking
**After**: Session tokens are protected by TLS and cannot be sent over HTTP

This fix addresses the vulnerability by:
1. Preventing browsers from sending session cookies over HTTP (Secure attribute)
2. Redirecting all HTTP traffic to HTTPS at the edge (nginx)
3. Maintaining backward compatibility with local development workflows
