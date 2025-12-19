# botbin CLI Usage Guide

Upload and share files via `curl`. All uploads require an API token.

## Basic Upload

```bash
curl -X POST https://botbin.net/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@myfile.txt"
```

Returns the URL to your paste (e.g., `https://botbin.net/Av34X`).

## Custom Retention

Set how long the file stays (1 minute to 1 month). Default: 1 week.

```bash
curl -X POST https://botbin.net/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@screenshot.png" \
  -F "retention=2h30m"
```

Supported formats: `1m`, `5m`, `30m`, `1h`, `2h30m`, `24h`, `72h`, `168h`, `720h`

> Cleanup runs every 30 seconds, so actual deletion may vary slightly.

## Password Protection

Require a password to view the paste:

```bash
curl -X POST https://botbin.net/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@secret.txt" \
  -F "password=mysecret"
```

Access with: `https://botbin.net/Av34X?password=mysecret`

## Encrypted Upload

Encrypt content at rest (requires password):

```bash
curl -X POST https://botbin.net/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@private.txt" \
  -F "password=mysecret" \
  -F "encrypted=true"
```

Content is encrypted server-side before storage. Only decrypted when accessed with the correct password.

## All Options Combined

```bash
curl -X POST https://botbin.net/upload \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@document.pdf" \
  -F "retention=72h" \
  -F "password=mysecret" \
  -F "encrypted=true"
```

## Supported File Types

- **Images:** JPEG, PNG, GIF, WebP, SVG
- **Video:** MP4, WebM, OGG
- **Audio:** MP3, WAV, OGG
- **Text:** Plain text, HTML, CSS, JS, Markdown, JSON, XML

**Blocked:** Executables and archives (security)

## Limits

- Max file size: 20 MB
- Retention: 1 minute to 1 month
- Requires approved API token

## Getting a Token

1. Sign up at https://botbin.net
2. Request a token from your dashboard
3. Wait for admin approval
4. Token is sent to your email
