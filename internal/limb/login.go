package limb

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/duo/octopus-qq/internal/common"

	"github.com/Mrs4s/MiraiGo/client"
	"github.com/Mrs4s/MiraiGo/utils"
	"github.com/mattn/go-colorable"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"gopkg.ilharper.com/x/isatty"

	log "github.com/sirupsen/logrus"
)

var (
	console = bufio.NewReader(os.Stdin)

	ErrSMSRequestError = errors.New("sms request error")
)

func (b *Bot) commonLogin() error {
	res, err := b.client.Login()
	if err != nil {
		return err
	}
	return b.loginResponseProcessor(res)
}

func (b *Bot) qrcodeLogin() error {
	rsp, err := b.client.FetchQRCodeCustomSize(1, 2, 1)
	if err != nil {
		return err
	}
	_ = os.WriteFile("qrcode.png", rsp.ImageData, 0o644)
	defer func() { _ = os.Remove("qrcode.png") }()
	log.Infof("Please use mobile phone scan qr code (qrcode.png): ")
	time.Sleep(time.Second)
	printQRCode(rsp.ImageData)
	s, err := b.client.QueryQRCodeStatus(rsp.Sig)
	if err != nil {
		return err
	}
	prevState := s.State
	for {
		time.Sleep(time.Second)
		s, _ = b.client.QueryQRCodeStatus(rsp.Sig)
		if s == nil {
			continue
		}
		if prevState == s.State {
			continue
		}
		prevState = s.State
		switch s.State {
		case client.QRCodeCanceled:
			log.Fatalf("Canceled")
		case client.QRCodeTimeout:
			log.Fatalf("Expired")
		case client.QRCodeWaitingForConfirm:
			log.Infof("Scan successful, please confirm by mobile phone")
		case client.QRCodeConfirmed:
			res, err := b.client.QRCodeLogin(s.LoginInfo)
			if err != nil {
				return err
			}
			return b.loginResponseProcessor(res)
		case client.QRCodeImageFetch, client.QRCodeWaitingForScan:
			// ignore
		}
	}
}

func (b *Bot) loginResponseProcessor(res *client.LoginResponse) error {
	var err error
	for {
		if err != nil {
			return err
		}
		if res.Success {
			return nil
		}
		var text string
		switch res.Error {
		case client.SliderNeededError:
			log.Warnf("Slidebar verify required: ")
			log.Warnf("1. Use browser")
			log.Warnf("2. Use mobile phone scan code")
			log.Warn("Input(1 - 2): ")
			text = readIfTTY("1")
			if strings.Contains(text, "1") {
				ticket := getTicket(res.VerifyUrl)
				if ticket == "" {
					os.Exit(0)
				}
				res, err = b.client.SubmitTicket(ticket)
				continue
			}
			return b.qrcodeLogin()
		case client.NeedCaptcha:
			log.Warnf("Cpatcha verify required: ")
			_ = os.WriteFile("captcha.jpg", res.CaptchaImage, 0o644)
			log.Warnf("Enter captcha (captcha.jpg): (submit on enter)")
			text = readLine()
			common.DelFile("captcha.jpg")
			res, err = b.client.SubmitCaptcha(text, res.CaptchaSign)
			continue
		case client.SMSNeededError:
			log.Warnf("Device locked,, press enter send SMS to %v", res.SMSPhone)
			readLine()
			if !b.client.RequestSMS() {
				log.Warnf("Send SMS failed")
				return errors.WithStack(ErrSMSRequestError)
			}
			log.Warn("Input SMS code: (submit on enter)")
			text = readLine()
			res, err = b.client.SubmitSMS(text)
			continue
		case client.SMSOrVerifyNeededError:
			log.Warnf("Device locked:")
			log.Warnf("1. send SMS to %v", res.SMSPhone)
			log.Warnf("2. Use mobile phone scan code")
			log.Warn("Input(1 - 2): ")
			text = readIfTTY("2")
			if strings.Contains(text, "1") {
				if !b.client.RequestSMS() {
					log.Warnf("Send SMS failed")
					return errors.WithStack(ErrSMSRequestError)
				}
				log.Warn("Input SMS code: (submit on enter)")
				text = readLine()
				res, err = b.client.SubmitSMS(text)
				continue
			}
			fallthrough
		case client.UnsafeDeviceError:
			log.Warnf("Device locked, go to > %v <-", res.VerifyUrl)
			os.Exit(0)
		case client.OtherLoginError, client.UnknownLoginError, client.TooManySMSRequestError:
			msg := res.ErrorMessage
			log.Warnf("Login failed: %v", msg)
			os.Exit(0)
		}
	}
}

func getTicket(u string) (str string) {
	id := utils.RandomString(8)
	log.Warnf("Verify ticket %v", strings.ReplaceAll(u, "https://ssl.captcha.qq.com/template/wireless_mqq_captcha.html?", fmt.Sprintf("https://captcha.go-cqhttp.org/captcha?id=%v&", id)))
	manual := make(chan string, 1)
	go func() {
		manual <- readLine()
	}()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for count := 120; count > 0; count-- {
		select {
		case <-ticker.C:
			str = fetchCaptcha(id)
			if str != "" {
				return
			}
		case str = <-manual:
			return
		}
	}
	log.Warnf("Verify ticket expired")
	return ""
}

func fetchCaptcha(id string) string {
	data, err := common.GetBytes("https://captcha.go-cqhttp.org/captcha/ticket?id=" + id)
	if err != nil {
		log.Debugln("Failed to get verify ticket", err)
		return ""
	}
	g := gjson.ParseBytes(data)
	if g.Get("ticket").Exists() {
		return g.Get("ticket").String()
	}
	return ""
}

func printQRCode(imgData []byte) {
	const (
		black = "\033[48;5;0m  \033[0m"
		white = "\033[48;5;7m  \033[0m"
	)
	img, err := png.Decode(bytes.NewReader(imgData))
	if err != nil {
		log.Panic(err)
	}
	data := img.(*image.Gray).Pix
	bound := img.Bounds().Max.X
	buf := make([]byte, 0, (bound*4+1)*(bound))
	i := 0
	for y := 0; y < bound; y++ {
		i = y * bound
		for x := 0; x < bound; x++ {
			if data[i] != 255 {
				buf = append(buf, white...)
			} else {
				buf = append(buf, black...)
			}
			i++
		}
		buf = append(buf, '\n')
	}
	_, _ = colorable.NewColorableStdout().Write(buf)
}

func readLine() (str string) {
	str, _ = console.ReadString('\n')
	str = strings.TrimSpace(str)
	return
}

func readIfTTY(de string) (str string) {
	if isatty.Isatty(os.Stdin.Fd()) {
		return readLine()
	}
	log.Warnf("Input not detected, chose %s.", de)
	return de
}

func energy(signServer string, uin uint64, id string, appVersion string, salt []byte) ([]byte, error) {
	if !strings.HasSuffix(signServer, "/") {
		signServer += "/"
	}

	response, err := common.Request{
		Method: http.MethodGet,
		URL:    signServer + "custom_energy" + fmt.Sprintf("?data=%v&salt=%v", id, hex.EncodeToString(salt)),
	}.Bytes()
	if err != nil {
		log.Errorf("Failed to fetch T544: %v", err)
		return nil, err
	}

	data, err := hex.DecodeString(gjson.GetBytes(response, "data").String())
	if err != nil {
		log.Errorf("Failed to fetch T544: %v", err)
		return nil, err
	}
	if len(data) == 0 {
		return nil, errors.New("data is empty")
	}

	return data, nil
}

func sign(signServer string, seq uint64, uin string, cmd string, qua string, buff []byte) (sign []byte, extra []byte, token []byte, err error) {
	if !strings.HasSuffix(signServer, "/") {
		signServer += "/"
	}

	response, err := common.Request{
		Method: http.MethodPost,
		URL:    signServer + "sign",
		Header: map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
		Body:   bytes.NewReader([]byte(fmt.Sprintf("uin=%v&qua=%s&cmd=%s&seq=%v&buffer=%v", uin, qua, cmd, seq, hex.EncodeToString(buff)))),
	}.Bytes()
	if err != nil {
		return nil, nil, nil, err
	}

	sign, _ = hex.DecodeString(gjson.GetBytes(response, "data.sign").String())
	extra, _ = hex.DecodeString(gjson.GetBytes(response, "data.extra").String())
	token, _ = hex.DecodeString(gjson.GetBytes(response, "data.token").String())

	return sign, extra, token, nil
}
