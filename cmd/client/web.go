package main

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

func panicIf(err error) {
	if err != nil {
		panic(err)
	}
}

type any = interface{}
type jObject = map[string]any
type jArray = []any

var gPathLead string
var gCookies string
var gExtraQs string
var urlBase string
var client = &http.Client{}

func requestJson(path string, qs string) any {
	req, err := http.NewRequest("GET", getUrl(path, qs), nil)
	panicIf(err)
	req.Header.Add("Cookie", gCookies)
	resp, err := client.Do(req)
	panicIf(err)
	var v any
	err = json.NewDecoder(resp.Body).Decode(&v)
	if err != nil {
		panic(err)
	}
	return v
}

func getUrl(path string, qs string) string {
	return urlBase + path + "?" + gExtraQs + qs
}

func getUserId(targetId string) string {
	ra := requestJson("/api/v1/perms/users/assets/" + targetId + "/system-users/", "cache_policy=1").(jArray)
	ro := ra[0].(jObject)
	return ro["id"].(string)
}

func getTargetId() string {
	ro := requestJson("/api/v1/perms/users/assets/", "offset=0&limit=15&display=1&draw=1").(jObject)
	ra := ro["results"].(jArray)
	ro = ra[0].(jObject)
	return ro["id"].(string)
}

func getWsConn(host string, cookies string, extraQs string) *websocket.Conn {
	gCookies = cookies
	gExtraQs = extraQs + "&"
	hu, err := url.Parse(host)
	panicIf(err)
	if hu.Host == "" {
		hu, err = url.Parse("https://" + host)
		panicIf(err)
	}
	wsPath := "/koko/ws/terminal/"
	urlBase = "https://" + hu.Host
	if hu.Path != "" {
		lead := strings.TrimSuffix(hu.Path, "/")
		urlBase += lead
		wsPath = lead + wsPath
	}

	targetId := getTargetId()
	userId := getUserId(targetId)
	u := url.URL{Scheme: "wss", Host: hu.Host, Path: strings.Replace(wsPath, "http", "ws", -1), RawQuery: "target_id=" + targetId + "&type=asset&system_user_id=" + userId}
	rawUrl := u.String()

	h := http.Header{}
	h.Set("Cookie", cookies)

	websocket.DefaultDialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	log.Print("正在连接... ", rawUrl)
	conn, _, err := websocket.DefaultDialer.Dial(rawUrl, h)
	if err != nil {
		panic("连接失败，告辞。" + err.Error())
	}
	return conn
}
