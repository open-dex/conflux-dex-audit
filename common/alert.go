package common

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

// DingDingAccessToken is the access token to send message to Dingding.
var DingDingAccessToken string

// InPauseTimeRange check whether current Shanghai time is between 7 a.m. and 11 p.m.
func InPauseTimeRange() bool {
	now := time.Now()
	local, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	nowTime := now.In(local)
	return nowTime.Hour() < 7 || nowTime.Hour() > 22
}

// AlertMatchflow signal a fatal error to DEX.
func AlertMatchflow() {
	if !InPauseTimeRange() {
		Alert("matchflow", "current time is between 7:00 a.m. and 11 p.m., check it manually!")
	} else {
		Alert("matchflow", "current time is out of 7:00 a.m. and 11 p.m., try to pause system!")
		timestamp := strconv.FormatInt(time.Now().UnixNano(), 10)
		l := len(timestamp)
		timestamp = timestamp[:l-6]
		toSign := []string{"dex.suspend", "", timestamp}
		rlpEncode, err := rlp.EncodeToBytes(toSign)
		if err != nil {
			panic(err)
		}
		privKey, err := crypto.HexToECDSA(AesDecrypt(DexAdminPrivKey, AesSecret))
		if err != nil {
			panic(err)
		}
		signature, err := crypto.Sign(crypto.Keccak256(rlpEncode), privKey)
		if err != nil {
			panic(err)
		}
		hexSig := hex.EncodeToString(signature)
		if hexSig[129:] == "0" {
			hexSig = "0x" + hexSig[:128] + "1b"
		} else {
			hexSig = "0x" + hexSig[:128] + "1c"
		}

		mapData := map[string]string{
			"command":   "dex.suspend",
			"comment":   "",
			"timestamp": timestamp,
			"signature": hexSig,
		}
		mapB, _ := json.Marshal(mapData)
		fmt.Println(string(mapB))
		response, err := http.Post(MatchflowURL+"/system/suspend", "application/json", strings.NewReader(string(mapB)))
		if err != nil {
			panic(err)
		}
		defer response.Body.Close()

		// read the payload, in this case, Jhon's info
		body, err := ioutil.ReadAll(response.Body)

		if err != nil {
			panic(err)
		}
		fmt.Println(string(body))
	}
	fmt.Println("Alert! Signal Pause to MatchFlow")
}

// AlertShuttleflow signal a fatal error to DEX.
func AlertShuttleflow() {
	if !InPauseTimeRange() {
		Alert("shuttleflow", "current time is between 7:00 a.m. and 11 p.m., check it manually!")
	} else {
		Alert("shuttleflow", "current time is out of 7:00 a.m. and 11 p.m., try to pause system!")
		var stdOut, stdErr bytes.Buffer
		cmd := exec.Command("ssh", CustodianAddress, "bash stop.sh")
		cmd.Stdout = &stdOut
		cmd.Stderr = &stdErr
		if err := cmd.Run(); err != nil {
			fmt.Printf("cmd exec failed: %s : %s", fmt.Sprint(err), stdErr.String())
		}
	}
	fmt.Println("Alert! Signal Pause to ShuttleFlow")
}

// Alert sends a message of specific module to Dingding.
func Alert(module, message string) {
	sendMessageToDingDing("[Alert] [%v] %v", module, message)
}

// Alertf sends a message of specific module to Dingding.
func Alertf(module, format string, a ...interface{}) {
	Alert(module, fmt.Sprintf(format, a...))
}

// Notify sends a message to Dingding.
func Notify(message string) {
	sendMessageToDingDing("[Notify] %v", message)
}

// Notifyf sends a message to Dingding.
func Notifyf(format string, a ...interface{}) {
	Notify(fmt.Sprintf(format, a...))
}

// SendMessageToDingDing sends a message to Dingding.
func sendMessageToDingDing(format string, a ...interface{}) {
	if len(DingDingAccessToken) == 0 {
		logrus.Warn("Acess token of Dingding is not configured!")
		return
	}

	client := resty.New()

	body := map[string]interface{}{
		"msgtype": "text",
		"text": map[string]interface{}{
			"content": fmt.Sprintf(format, a...),
		},
		"at": map[string]interface{}{
			"isAtAll": true,
		},
	}

	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post("https://oapi.dingtalk.com/robot/send?access_token=" + DingDingAccessToken)

	if err != nil {
		logrus.WithError(err).Error("failed to send message to Dingding")
		return
	}

	if !response.IsSuccess() {
		logrus.WithField("response", *response).Error("sending message to Dingding is not successful")
	}
}
