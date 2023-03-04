package limb

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/duo/octopus-qq/internal/common"

	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/message"
	"github.com/antchfx/xmlquery"
	"github.com/tidwall/gjson"

	log "github.com/sirupsen/logrus"
)

const (
	deviceFile   = "device.json"
	sessionToken = "session.token"

	retryMax      = 21
	retryInterval = 7 * time.Second
)

var smallestImg = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
	0x49, 0x46, 0x00, 0x01, 0x01, 0x01, 0x00, 0x48,
	0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
	0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xC2, 0x00, 0x0B, 0x08, 0x00, 0x01,
	0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4,
	0x00, 0x14, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xFF, 0xDA, 0x00, 0x08,
	0x01, 0x01, 0x00, 0x01, 0x3F, 0x10,
}

type Bot struct {
	config *common.Configure

	client *client.QQClient
	token  []byte

	pushFunc func(*common.OctopusEvent)

	stopSync chan struct{}
}

func (b *Bot) Login() {
	device := &client.DeviceInfo{}
	if !common.FileExist(deviceFile) {
		device = client.GenRandomDevice()
		if err := os.WriteFile(deviceFile, device.ToJson(), 0o644); err != nil {
			panic(err)
		}
	} else {
		if deviceInfo, err := os.ReadFile(deviceFile); err != nil {
			panic(err)
		} else if err := device.ReadJson(deviceInfo); err != nil {
			panic(err)
		}
	}
	b.client.UseDevice(device)

	isQRCodeLogin := b.config.Limb.Account == 0 || b.config.Limb.Password == ""
	isTokenLogin := false

	saveToken := func() {
		b.token = b.client.GenToken()
		_ = os.WriteFile(sessionToken, b.token, 0o644)
	}

	if common.FileExist(sessionToken) {
		token, err := os.ReadFile(sessionToken)
		if err == nil {
			if err := b.client.TokenLogin(token); err != nil {
				_ = os.Remove(sessionToken)
				log.Warnf("Restore session failed: %v", err)
				time.Sleep(time.Second)
				b.client.Disconnect()
				b.client.Release()
				b.client = client.NewClientEmpty()
			} else {
				isTokenLogin = true
				log.Infoln("Login by session successful")
			}
		}
	}

	if b.config.Limb.Account != 0 && b.config.Limb.Password != "" {
		b.client.Uin = b.config.Limb.Account
		b.client.PasswordMd5 = md5.Sum([]byte(b.config.Limb.Password))
	}

	if !isTokenLogin {
		if isQRCodeLogin {
			if err := b.qrcodeLogin(); err != nil {
				log.Fatalln("Login fatal:", err)
			}
		} else {
			if err := b.commonLogin(); err != nil {
				log.Fatalln("Login fatal:", err)
			}
		}
	}

	b.client.PrivateMessageEvent.Subscribe(b.processPrivateMessage)
	b.client.GroupMessageEvent.Subscribe(b.processGroupMessage)
	if b.config.Limb.HookSelf {
		b.client.SelfPrivateMessageEvent.Subscribe(b.processPrivateMessage)
		b.client.SelfGroupMessageEvent.Subscribe(b.processGroupMessage)
	}
	b.client.TempMessageEvent.Subscribe(b.processTempMessage)
	b.client.OfflineFileEvent.Subscribe(b.processOfflineFileEvent)
	b.client.FriendMessageRecalledEvent.Subscribe(b.processFriendRecalled)
	b.client.GroupMessageRecalledEvent.Subscribe(b.processGroupRecalled)

	var times uint = 1
	var reLoginLock sync.Mutex
	b.client.DisconnectedEvent.Subscribe(func(q *client.QQClient, e *client.ClientDisconnectedEvent) {
		reLoginLock.Lock()
		defer reLoginLock.Unlock()
		times = 1
		if b.client.Online.Load() {
			return
		}

		log.Warnf("Bot offline: %v", e.Message)
		time.Sleep(time.Second * time.Duration(5))
		for {
			if times > retryMax {
				log.Fatalf("Bot retry reach limit")
			}

			times++
			log.Warnf("Reconnect after %s, count: %d/%d", retryInterval, times, retryMax)
			time.Sleep(retryInterval)

			if b.client.Online.Load() {
				log.Infof("Reconnect successful")
				break
			}

			log.Warnln("Reconnecting...")
			if err := b.client.TokenLogin(b.token); err == nil {
				saveToken()
				return
			}

			log.Fatalln("Reconnect failed")
		}
	})

	saveToken()
	b.client.AllowSlider = true

	b.client.ReloadFriendList()
	b.client.ReloadGroupList()

	b.config.Limb.Account = b.client.Uin
}

func (b *Bot) Start() {
	log.Infoln("Bot started")

	go func() {
		time.Sleep(b.config.Service.SyncDelay)
		go b.sync()

		clock := time.NewTicker(b.config.Service.SyncInterval)
		defer func() {
			log.Infoln("LimbService sync stopped")
			clock.Stop()
		}()
		log.Infof("Syncing LimbService every %s", b.config.Service.SyncInterval)
		for {
			select {
			case <-clock.C:
				go b.sync()
			case <-b.stopSync:
				return
			}
		}
	}()
}

func (b *Bot) Stop() {
	log.Infoln("Bot stopping")

	select {
	case b.stopSync <- struct{}{}:
	default:
	}

	b.client.Disconnect()
	b.client.Release()
}

func NewBot(config *common.Configure, pushFunc func(*common.OctopusEvent)) *Bot {
	return &Bot{
		config:   config,
		client:   client.NewClientEmpty(),
		pushFunc: pushFunc,
		stopSync: make(chan struct{}),
	}
}

func (b *Bot) sync() {
	event := b.generateEvent("sync", time.Now().UnixMilli())

	chats := []*common.Chat{}

	chats = append(chats, &common.Chat{
		ID:    common.Itoa(b.client.Uin),
		Type:  "private",
		Title: b.client.Nickname,
	})
	for _, f := range b.client.FriendList {
		chats = append(chats, &common.Chat{
			ID:    common.Itoa(f.Uin),
			Type:  "private",
			Title: f.Remark,
		})
	}
	for _, g := range b.client.GroupList {
		chats = append(chats, &common.Chat{
			ID:    common.Itoa(g.Code),
			Type:  "group",
			Title: g.Name,
		})
	}

	event.Type = common.EventSync
	event.Data = chats

	b.pushFunc(event)
}

// process events from master
func (b *Bot) processOcotopusEvent(event *common.OctopusEvent) (*common.OctopusEvent, error) {
	log.Debugf("Receive Octopus event: %+v", event)

	target, err := common.Atoi(event.Chat.ID)
	if err != nil {
		return nil, err
	}

	var source message.Source
	if event.Chat.Type == "private" {
		source = message.Source{
			SourceType: message.SourcePrivate,
			PrimaryID:  target,
		}
	} else {
		source = message.Source{
			SourceType: message.SourceGroup,
			PrimaryID:  target,
		}
	}

	elems := []message.IMessageElement{}

	if event.Reply != nil {
		if key, err := EventKeyFromString(event.Reply.ID); err != nil {
			return nil, err
		} else {
			sender, err := common.Atoi(event.Reply.Sender)
			if err != nil {
				return nil, err
			}
			e := &message.ReplyElement{
				Sender:   sender,
				Time:     int32(event.Reply.Timestamp),
				Elements: []message.IMessageElement{message.NewText(event.Reply.Content)},
			}
			if event.Chat.Type == "private" {
				e.ReplySeq = int32(uint16(int32(key.IntSeq())))
			} else {
				e.ReplySeq = int32(key.IntSeq())
				e.GroupID = target
			}
			elems = append(elems, e)
		}
	}

	switch event.Type {
	case common.EventText:
		elems = append(elems, message.NewText(event.Content))
	case common.EventPhoto:
		photos := event.Data.([]*common.BlobData)
		for _, photo := range photos {
			e, err := b.client.UploadImage(source, bytes.NewReader(photo.Binary))
			if err != nil {
				log.Warnf("Failed to upload image to %v: %v", source, err)
				continue
			}
			elems = append(elems, e)
		}
	case common.EventSticker:
		blob := event.Data.(*common.BlobData)
		e, err := b.client.UploadImage(source, bytes.NewReader(blob.Binary))
		if err != nil {
			log.Warnf("Failed to upload image to %v: %v", source, err)
			return nil, err
		}
		elems = append(elems, e)
	case common.EventVideo:
		blob := event.Data.(*common.BlobData)
		e, err := b.client.UploadShortVideo(source, bytes.NewReader(blob.Binary), bytes.NewReader(smallestImg))
		if err != nil {
			log.Warnf("Failed to upload short video to %v: %v", source, err)
			return nil, err
		}
		elems = append(elems, e)
	case common.EventAudio:
		blob := event.Data.(*common.BlobData)
		e, err := b.client.UploadVoice(source, bytes.NewReader(blob.Binary))
		if err != nil {
			log.Warnf("Failed to upload voice to %v: %v", source, err)
			return nil, err
		}
		elems = append(elems, e)
	case common.EventFile:
		blob := event.Data.(*common.BlobData)
		f := &client.LocalFile{
			FileName:     blob.Name,
			Body:         bytes.NewReader(blob.Binary),
			RemoteFolder: "/",
		}
		if err := b.client.UploadFile(source, f); err != nil {
			log.Warnf("Failed to upload file to %v: %v", source, err)
			return nil, err
		}
	case common.EventLocation:
		location := event.Data.(*common.LocationData)
		locationJson := fmt.Sprintf(`
		{
			"app": "com.tencent.map",
			"desc": "地图",
			"view": "LocationShare",
			"ver": "0.0.0.1",
			"prompt": "[应用]地图",
			"from": 1,
			"meta": {
			  "Location.Search": {
				"id": "12250896297164027526",
				"name": "%s",
				"address": "%s",
				"lat": "%.5f",
				"lng": "%.5f",
				"from": "plusPanel"
			  }
			},
			"config": {
			  "forward": 1,
			  "autosize": 1,
			  "type": "card"
			}
		}
		`, location.Name, location.Address, location.Latitude, location.Longitude)
		elems = append(elems, message.NewLightApp(locationJson))
	case common.EventRevoke:
		// TODO:
	default:
		return nil, fmt.Errorf("%s not support", event.Type)
	}

	targetMsg := &message.SendingMessage{Elements: elems}
	if event.Chat.Type == "private" {
		if ret := b.client.SendPrivateMessage(target, targetMsg); ret == nil {
			return nil, fmt.Errorf("failed to send private message to %v", source)
		} else {
			return &common.OctopusEvent{
				ID:        NewEventKey(ret.Id, ret.InternalId).String(),
				Timestamp: int64(ret.Time),
			}, nil
		}
	} else {
		if ret := b.client.SendGroupMessage(target, targetMsg); ret == nil {
			return nil, fmt.Errorf("failed to send group message to %v", source)
		} else {
			return &common.OctopusEvent{
				ID:        NewEventKey(ret.Id, ret.InternalId).String(),
				Timestamp: int64(ret.Time),
			}, nil
		}
	}
}

func (b *Bot) processPrivateMessage(c *client.QQClient, m *message.PrivateMessage) {
	eventKey := NewEventKey(m.Id, m.InternalId)
	event := b.generateEvent(eventKey.String(), int64(m.Time))

	targetID := m.Target
	if m.Target == b.client.Uin {
		targetID = m.Sender.Uin
	}
	target := b.client.FindFriend(targetID)
	var targetName string
	if target != nil {
		if len(target.Remark) == 0 {
			targetName = target.Remark
		} else {
			targetName = target.Nickname
		}
	} else {
		targetName = common.Itoa(targetID)
	}

	event.From = common.User{
		ID:       common.Itoa(m.Sender.Uin),
		Username: m.Sender.Nickname,
		Remark:   m.Sender.CardName,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(targetID),
		Title: targetName,
	}

	b.processEvent(event, m.Elements)
}

func (b *Bot) processGroupMessage(c *client.QQClient, m *message.GroupMessage) {
	eventKey := NewEventKey(m.Id, m.InternalId)
	event := b.generateEvent(eventKey.String(), int64(m.Time))

	event.From = common.User{
		ID:       common.Itoa(m.Sender.Uin),
		Username: m.Sender.Nickname,
		Remark:   m.Sender.CardName,
	}
	event.Chat = common.Chat{
		Type:  "group",
		ID:    common.Itoa(m.GroupCode),
		Title: m.GroupName,
	}

	b.processEvent(event, m.Elements)
}

func (b *Bot) processTempMessage(c *client.QQClient, e *client.TempMessageEvent) {
	eventKey := NewPartialKey(int64(e.Message.Id))
	event := b.generateEvent(eventKey.String(), time.Now().UnixMilli())

	m := e.Message
	username := m.Sender.Nickname

	info, err := b.client.GetSummaryInfo(m.Sender.Uin)
	if err != nil {
		log.Warnf("Get summary info failed: %v", err)
	} else {
		if len(m.GroupName) == 0 {
			username = info.Nickname + "(nil)"
		} else {
			username = info.Nickname + "(" + m.GroupName + ")"
		}
	}

	event.From = common.User{
		ID:       common.Itoa(m.Sender.Uin),
		Username: username,
		Remark:   username,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(m.Sender.Uin),
		Title: username,
	}

	b.processEvent(event, e.Message.Elements)
}

func (b *Bot) processOfflineFileEvent(c *client.QQClient, e *client.OfflineFileEvent) {
	eventKey := NewPartialKey(time.Now().Unix())
	event := b.generateEvent(eventKey.String(), time.Now().UnixMilli())

	f := b.client.FindFriend(e.Sender)
	if f == nil {
		log.Warnf("Failed to lookup file sender: %d", e.Sender)
		return
	}

	event.From = common.User{
		ID:       common.Itoa(f.Uin),
		Username: f.Nickname,
		Remark:   f.Remark,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(b.client.Uin),
		Title: b.client.Nickname,
	}

	blob, err := common.Download(e.DownloadUrl)
	if err != nil {
		event.Type = common.EventText
		event.Content = "[文件下载失败]"
	} else {
		blob.Name = e.FileName
		event.Type = common.EventFile
		event.Data = blob
	}

	b.pushFunc(event)
}

func (b *Bot) processFriendRecalled(c *client.QQClient, e *client.FriendMessageRecalledEvent) {
	eventKey := NewPartialKey(time.Now().Unix())
	event := b.generateEvent(eventKey.String(), time.Now().UnixMilli())

	f := b.client.FindFriend(e.FriendUin)
	if f == nil {
		log.Warnf("Failed to lookup friend recall operator: %d", e.FriendUin)
		return
	}

	event.From = common.User{
		ID:       common.Itoa(f.Uin),
		Username: f.Nickname,
		Remark:   f.Remark,
	}
	event.Chat = common.Chat{
		Type:  "private",
		ID:    common.Itoa(f.Uin),
		Title: f.Remark,
	}

	event.Type = common.EventRevoke
	event.Content = "recalled a message"

	event.Reply = &common.ReplyInfo{
		ID:        common.Itoa(int64(e.MessageId)),
		Timestamp: int64(e.Time),
		Sender:    common.Itoa(e.FriendUin),
	}

	b.pushFunc(event)
}

func (b *Bot) processGroupRecalled(c *client.QQClient, e *client.GroupMessageRecalledEvent) {
	eventKey := NewPartialKey(time.Now().Unix())
	event := b.generateEvent(eventKey.String(), time.Now().UnixMilli())

	var m *client.GroupMemberInfo
	group := b.client.FindGroup(e.GroupCode)
	if group != nil {
		if members, err := b.client.GetGroupMembers(group); err == nil {
			for _, member := range members {
				if member.Uin == e.AuthorUin {
					m = member
				}
			}
		}
	}
	if m == nil {
		log.Warnf("Failed to lookup group recall operator: %d", e.AuthorUin)
		return
	}

	event.From = common.User{
		ID:       common.Itoa(m.Uin),
		Username: m.Nickname,
		Remark:   m.CardName,
	}
	event.Chat = common.Chat{
		Type:  "group",
		ID:    common.Itoa(e.GroupCode),
		Title: group.Name,
	}

	event.Type = common.EventRevoke
	event.Content = "recalled a message"

	event.Reply = &common.ReplyInfo{
		ID:        common.Itoa(int64(e.MessageId)),
		Timestamp: int64(e.Time),
		Sender:    common.Itoa(e.AuthorUin),
	}

	b.pushFunc(event)
}

func (b *Bot) processEvent(event *common.OctopusEvent, elems []message.IMessageElement) {
	event.Type = common.EventText

	var summary []string

	photos := []*common.BlobData{}
	for _, e := range elems {
		switch v := e.(type) {
		case *message.TextElement:
			summary = append(summary, v.Content)
		case *message.FaceElement:
			summary = append(summary, common.ConvertFace(v.Name))
		case *message.AtElement:
			summary = append(summary, v.Display)
		case *message.FriendImageElement:
			summary = append(summary, "[图片]")

			if bin, err := common.Download(v.Url); err != nil {
				log.Warnf("Download friend image failed: %v", err)
			} else {
				bin.Name = v.ImageId
				photos = append(photos, bin)
			}
		case *message.GroupImageElement:
			summary = append(summary, "[图片]")

			if bin, err := common.Download(v.Url); err != nil {
				log.Warnf("Download group image failed: %v", err)
			} else {
				bin.Name = v.ImageId
				photos = append(photos, bin)
			}
		case *message.ShortVideoElement:
			url := b.client.GetShortVideoUrl(v.Uuid, v.Md5)
			if bin, err := common.Download(url); err != nil {
				log.Warnf("Download video failed: %v", err)
				event.Content = "[视频下载失败]"
			} else {
				bin.Name = v.Name
				event.Type = common.EventVideo
				event.Data = bin
			}
		case *message.GroupFileElement:
			groupCode, _ := common.Atoi(event.Chat.ID)
			url := b.client.GetGroupFileUrl(groupCode, v.Path, v.Busid)
			if bin, err := common.Download(url); err == nil {
				bin.Name = v.Name
				event.Type = common.EventFile
				event.Data = bin
			} else {
				log.Warnf("Download group file failed: %v", err)
				event.Content = "[文件下载失败]"
			}
		case *message.VoiceElement:
			if bin, err := common.Download(v.Url); err == nil {
				event.Type = common.EventAudio
				event.Data = bin
			} else {
				log.Warnf("Download voice failed: %v", err)
				event.Content = "[语音下载失败]"
			}
		case *message.ReplyElement:
			event.Reply = &common.ReplyInfo{
				ID:        common.Itoa(int64(v.ReplySeq)),
				Timestamp: int64(v.Time),
				Sender:    common.Itoa(v.Sender),
			}
		case *message.LightAppElement:
			// TODO:
			view := gjson.Get(v.Content, "view").String()
			if view == "LocationShare" {
				name := gjson.Get(v.Content, "meta.*.name").String()
				address := gjson.Get(v.Content, "meta.*.address").String()
				latitude := gjson.Get(v.Content, "meta.*.lat").Float()
				longitude := gjson.Get(v.Content, "meta.*.lng").Float()
				event.Type = common.EventLocation
				event.Data = &common.LocationData{
					Name:      name,
					Address:   address,
					Longitude: longitude,
					Latitude:  latitude,
				}
			} else {
				if url := gjson.Get(v.Content, "meta.*.qqdocurl").String(); len(url) > 0 {
					title := gjson.Get(v.Content, "meta.*.title").String()
					desc := gjson.Get(v.Content, "meta.*.desc").String()
					prompt := gjson.Get(v.Content, "prompt").String()
					event.Type = common.EventApp
					event.Data = &common.AppData{
						Title:       prompt,
						Description: desc,
						Source:      title,
						URL:         url,
					}
				} else if jumpUrl := gjson.Get(v.Content, "meta.*.jumpUrl").String(); len(jumpUrl) > 0 {
					//title := gjson.Get(v.Content, "meta.*.title").String()
					desc := gjson.Get(v.Content, "meta.*.desc").String()
					prompt := gjson.Get(v.Content, "prompt").String()
					tag := gjson.Get(v.Content, "meta.*.tag").String()
					event.Type = common.EventApp
					event.Data = &common.AppData{
						Title:       prompt,
						Description: desc,
						Source:      tag,
						URL:         jumpUrl,
					}
				}
			}
		case *message.ForwardElement:
			event.Type = common.EventApp
			event.Data = b.convertForward(event.ID, v)
		case *message.AnimatedSticker:
			summary = append(summary, "/"+v.Name)
		case *message.MarketFaceElement:
			summary = append(summary, v.Name)
		default:
			summary = append(summary, fmt.Sprintf("[%v]", v.Type()))
		}
	}

	if len(summary) > 0 {
		if event.Reply != nil {
			summary = summary[1:]
		}

		if len(summary) == 1 && elems[0].Type() == message.Image {
			event.Type = common.EventPhoto
			event.Data = photos

			if v, ok := elems[0].(*message.GroupImageElement); ok {
				if v.ImageBizType == message.CustomFaceImage || v.ImageBizType == message.StickerImage {
					event.Type = common.EventSticker
					event.Data = photos[0]
				}
			}
		} else {
			event.Content = strings.Join(summary, "")

			if len(photos) > 0 {
				event.Type = common.EventPhoto
				event.Data = photos
			}
		}
	}

	b.pushFunc(event)
}

func (b *Bot) convertForward(id string, elem *message.ForwardElement) *common.AppData {
	var summary []string
	var content []string
	var blobs = map[string]*common.BlobData{}

	var handleForward func(level int, nodes []*message.ForwardNode)
	handleForward = func(level int, nodes []*message.ForwardNode) {
		summary = append(summary, "ForwardMessage:\n")
		if level > 0 {
			content = append(content, "<blockquote>")
		}

		for _, node := range nodes {
			name := node.SenderName
			if len(name) == 0 {
				name = common.Itoa(node.SenderId)
			}

			summary = append(summary, fmt.Sprintf("%s:\n", name))
			content = append(content, fmt.Sprintf("<strong>%s:</strong><p>", name))
			for _, e := range node.Message {
				switch v := e.(type) {
				case *message.TextElement:
					summary = append(summary, v.Content)
					content = append(content, v.Content)
				case *message.FaceElement:
					summary = append(summary, common.ConvertFace(v.Name))
					content = append(content, common.ConvertFace(v.Name))
				case *message.AtElement:
					summary = append(summary, v.Display)
					content = append(content, v.Display)
				case *message.FriendImageElement:
					summary = append(summary, "[图片]")
					if bin, err := common.Download(v.Url); err != nil {
						log.Warnf("Download forward friend image failed: %v", err)
						content = append(content, "[图片]")
					} else {
						md5 := hex.EncodeToString(v.Md5)
						bin.Name = v.ImageId
						blobs[md5] = bin
						content = append(content, fmt.Sprintf("<img src=\"%s%s\">", common.REMOTE_PREFIX, md5))
					}
				case *message.GroupImageElement:
					summary = append(summary, "[图片]")
					if bin, err := common.Download(v.Url); err != nil {
						log.Warnf("Download forward image failed: %v", err)
						content = append(content, "[图片]")
					} else {
						md5 := hex.EncodeToString(v.Md5)
						bin.Name = v.ImageId
						blobs[md5] = bin
						content = append(content, fmt.Sprintf("<img src=\"%s%s\">", common.REMOTE_PREFIX, md5))
					}
				case *message.ForwardMessage:
					handleForward(level+1, v.Nodes)
				case *message.ServiceElement:
					if v.SubType != "xml" {
						continue
					}
					doc, err := xmlquery.Parse(strings.NewReader(v.Content))
					if err != nil {
						log.Warnln("Failed to parse ServiceElement:", err)
						continue
					}
					resNode := xmlquery.FindOne(doc, "/msg/@m_resid")
					if resNode != nil && len(resNode.InnerText()) != 0 {
						msg := b.client.GetForwardMessage(resNode.InnerText())
						if msg != nil {
							handleForward(level+1, msg.Nodes)
						}
					} else {
						briefNode := xmlquery.FindOne(doc, "/msg/@brief")
						content = append(content, "<blockquote>")
						if briefNode != nil && len(briefNode.InnerText()) != 0 {
							summary = append(summary, fmt.Sprintf("%s:\n", briefNode.InnerText()))
							content = append(content, fmt.Sprintf("<strong>%s:</strong><p>", briefNode.InnerText()))
						} else {
							summary = append(summary, "Items:\n")
							content = append(content, "<strong>Items:</strong><p>")
						}
						for _, title := range xmlquery.Find(doc, "/msg/item/title") {
							summary = append(summary, title.InnerText()+"\n")
							content = append(content, "<p>", title.InnerText(), "</p>")
						}
						content = append(content, "</p></blockquote>")
					}
				case *message.AnimatedSticker:
					summary = append(summary, "/"+v.Name)
					content = append(content, "/"+v.Name)
				case *message.MarketFaceElement:
					summary = append(summary, v.Name)
					content = append(content, v.Name)
				default:
					summary = append(summary, fmt.Sprintf("[%v]", v.Type()))
					content = append(content, fmt.Sprintf("[%v]", v.Type()))
				}
			}
			summary = append(summary, "\n")
			content = append(content, "</p>")
		}

		if level > 0 {
			content = append(content, "</blockquote>")
		}
	}

	msg := b.client.GetForwardMessage(elem.ResId)
	if msg != nil {
		handleForward(0, msg.Nodes)
	} else {
		log.Info("Failed to get forward!")
	}

	return &common.AppData{
		Title:       fmt.Sprintf("[聊天记录 %s]", id),
		Description: strings.Join(summary, ""),
		Content:     strings.Join(content, ""),
		Blobs:       blobs,
	}
}

func (b *Bot) generateEvent(id string, ts int64) *common.OctopusEvent {
	return &common.OctopusEvent{
		Vendor:    b.getVendor(),
		ID:        id,
		Timestamp: ts,
	}
}

func (b *Bot) getVendor() common.Vendor {
	return common.Vendor{
		Type: "qq",
		UID:  common.Itoa(b.client.Uin),
	}
}
