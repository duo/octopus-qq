package limb

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/png"
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
			ticket := getTicket(res.VerifyUrl)
			if ticket == "" {
				os.Exit(0)
			}
			res, err = b.client.SubmitTicket(ticket)
			continue
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
			log.Warnf("Login failed: %v, code: %v", msg, res.Code)
			switch res.Code {
			case 235:
				log.Warnf("Device has been banned, please delete device.json and try again")
			case 237:
				log.Warnf("Login attempts are too frequent, wait for a while before trying again")
			case 45:
				log.Warnf("Account is restricted from logging in, please configure SignServer and try again")
			}
			os.Exit(0)
		}
	}
}

func getTicket(u string) (str string) {
	log.Warnf("Select slidebar ticket submit method:")
	log.Warnf("1. Auto")
	log.Warnf("2. Maunal")
	log.Warn("Input(1 - 2): ")
	text := readLine()

	id := utils.RandomString(8)
	auto := !strings.Contains(text, "2")
	if auto {
		u = strings.ReplaceAll(u, "https://ssl.captcha.qq.com/template/wireless_mqq_captcha.html?", fmt.Sprintf("https://captcha.go-cqhttp.org/captcha?id=%v&", id))
	}
	log.Warnf("Verify ticket -> %v", u)

	if !auto {
		log.Warn("Input ticket: (submit on enter)")
		return readLine()
	}

	for count := 120; count > 0; count-- {
		str := fetchCaptcha(id)
		if str != "" {
			return str
		}
		time.Sleep(time.Second)
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
