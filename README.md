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
  protocol: 6 # Optional, qq protocol (1: AndroidPhone, 2: AndroidWatch, 3: MacOS, 4: QiDian, 5: IPad, 6: AndroidPad)
  sign:
    server: "http://10.10.10.10:8080" # Optional, sign server address (https://github.com/fuqiuluo/unidbg-fetch-qsign)
    bearer: "" # Optional, sign server bearer token
    key: "114514" # Optional, sign server API key
    is_below_110: false # Optional, sign server version below 1.1.0
    refresh_interval: 30m # Optional, the interval time for scheduled token refreshing

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
