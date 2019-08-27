package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/bitly/go-simplejson"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Precipitation struct {
	date          time.Time
	precipitation float64
}

const (
	kyotoNo            = "61286"
	yearIndex          = 4
	monthIndex         = 5
	dayIndex           = 6
	hourIndex          = 7
	minuteIndex        = 8
	precipitationIndex = 9
	interval           = 40 // 観測から更新までが30分、実行間隔が10分
	slackUrlKey        = "SLACK_API_KEY"
)

func main() {
	lambda.Start(notifyPrecipitation)
}

func notifyPrecipitation() {
	slackUrl := os.Getenv(slackUrlKey)
	msg, err := getMessage()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "an error has occurred when fetching precipitation data")
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return
	}
	if msg == "" {
		_, _ = fmt.Fprintln(os.Stderr, "nothing to inform")
		return
	}

	fmt.Println(msg)
	json, err := createJsonBody(msg)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "an error has occurred when creating json body")
		_, _ = fmt.Fprintln(os.Stderr, err.Error())
		return
	}
	sendJson(json, slackUrl)
}

func sendJson(json []byte, slackUrl string) {
	_, _ = http.Post(slackUrl, "application/json", bytes.NewBuffer(json))

}

func createJsonBody(message string) ([]byte, error) {
	json := simplejson.New()
	json.Set("text", message)
	out := make([]byte, 0, len(message)*2)
	out, err := json.MarshalJSON()
	return out, err
}

func getMessage() (string, error) {
	response, err := http.Get("http://www.data.jma.go.jp/obd/stats/data/mdrr/pre_rct/alltable/pre1h00_rct.csv")
	if err != nil {
		return "", err
	}
	kyotoPrecipitation, err := extractKyotoPrecipitation(response.Body)
	if err != nil {
		return "", err
	}
	if kyotoPrecipitation == nil {
		return "", errors.New("指定の降水量データが見つかりませんでした")
	}
	currentTime := time.Now()
	if sub := currentTime.Sub(kyotoPrecipitation.date); sub.Minutes() > interval {
		return "", errors.New("指定の間隔より古いデータが見つかりました")
	}
	base := fmt.Sprintf("%02d:%02d〜の京都の一時間降水量は%.1fmmです\n",
		kyotoPrecipitation.date.Hour(),
		kyotoPrecipitation.date.Minute(),
		kyotoPrecipitation.precipitation)
	var additionalMessage string
	if kyotoPrecipitation.precipitation >= 80 {
		additionalMessage = "猛烈な雨です!"
	} else if kyotoPrecipitation.precipitation >= 50 {
		additionalMessage = "非常に激しい雨が降ります"
	} else if kyotoPrecipitation.precipitation >= 30 {
		additionalMessage = "激しい雨が降ります"
	} else if kyotoPrecipitation.precipitation >= 20 {
		additionalMessage = "強い雨が降ります"
	} else if kyotoPrecipitation.precipitation >= 10 {
		additionalMessage = "やや強い雨が降ります"
	} else if kyotoPrecipitation.precipitation >= 5 {
		additionalMessage = "やや本降りと言えます"
	} else if kyotoPrecipitation.precipitation >= 2 {
		additionalMessage = "傘が必要になるかもしれません"
	} else if kyotoPrecipitation.precipitation >= 1 {
		additionalMessage = "シトシトとした雨が降ります"
	} else {
		return "", nil
	}
	return base + additionalMessage, nil

}

func extractKyotoPrecipitation(reader io.Reader) (*Precipitation, error) {
	scanner := bufio.NewScanner(transform.NewReader(reader, japanese.ShiftJIS.NewDecoder()))
	for scanner.Scan() {
		line := scanner.Text()
		records := strings.Split(line, ",")
		if records[0] != kyotoNo {
			continue
		}

		givenTimeS := fmt.Sprintf("%s-%s-%sT%s:%s:00+09:00",
			records[yearIndex],
			records[monthIndex],
			records[dayIndex],
			records[hourIndex],
			records[minuteIndex])
		givenTime, err := time.Parse(time.RFC3339, givenTimeS)
		if err != nil {
			return nil, err
		}
		precipitation, err := strconv.ParseFloat(records[precipitationIndex], 64)
		return &Precipitation{
			date:          givenTime,
			precipitation: precipitation,
		}, nil

	}
	return nil, nil
}
