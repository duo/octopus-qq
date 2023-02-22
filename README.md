# Octopus QQ
Octopus QQ limb.

# Docker
* [octopus-qq](https://hub.docker.com/r/lxduo/octopus-qq)
```shell
docker run -d --name=octopus-qq --restart=always -v octopus-qq:/data lxduo/octopus-qq:latest
```

# Documentation

## Configuration
* configure.yaml
```yaml
limb:
  account: # Optional, QQ account (leave empty for QR code login)
  password: # Optional, QQ password (leave empty for QR code login)

service:
  addr: ws://10.10.10.10:11111 # Required, ocotpus address
  secret: hello # Reuqired, user defined secret
  ping_interval: 30s # Optional
  sync_delay: 1m # Optional
  sync_interval: 6h # Optional

log:
  level: info
```

## Feature

* Telegram → QQ
  * [ ] Message types
    * [x] Text
	* [x] Image
	* [x] Sticker
	* [x] Video
	* [x] Audio
    * [x] File
    * [ ] Mention
    * [x] Reply
    * [x] Location
  * [ ] Redaction

* QQ → Telegram
  * [ ] Message types
    * [x] Text
	* [x] Image
	* [x] Sticker
	* [x] Video
	* [x] Audio
    * [x] File
    * [ ] Mention
    * [x] Reply
    * [x] Location
  * [ ] Chat types
    * [x] Private
    * [x] Group
    * [ ] Stranger (unidirectional)
  * [x] Redaction
  * [x] Login types
	* [x] Password
	* [x] QR code
