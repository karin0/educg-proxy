package main

import (
	"bytes"
	"context"
	"edu/utils"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/xtaci/kcptun/generic"
	"github.com/xtaci/smux"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var wsId string

func write(w *websocket.Conn, content string, typ string) {
	a := make(map[string]string)
	// log.Print("Send ", typ, ": ", content)
	a["data"] = content
	a["id"] = wsId
	a["type"] = typ

	err := w.WriteJSON(a)
	if err != nil {
		panic(err)
	}
}

func writeStdin(w *websocket.Conn, content string) {
	write(w, content, "TERMINAL_DATA")
}

func writeInit(w *websocket.Conn, typ string) {
	write(w, "{\"cols\":69,\"rows\":36}", typ)
}

func writePing(w *websocket.Conn) {
	write(w, "", "PING")
}

func readGarbage(ch <-chan uint8, w *websocket.Conn) {
readGarbage:
	for {
		select {
		case <-ch:
		case <-time.After(500 * time.Millisecond):
			break readGarbage
		}
	}

	writeInit(w, "TERMINAL_RESIZE")
	/* concurrent writing breaks performance
	ticker := time.NewTicker(25 * time.Second)
	go func() {
		for {
			select {
			case <- ticker.C:
				writePing(w)
			}
		}
	}()
	 */
}

func websocketReader(ch chan<- uint8, conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			panic("读取出错，告辞。" + err.Error())
		}
		var j map[string]interface{}
		err = json.Unmarshal(message, &j)
		if err != nil {
			panic("JSON 解析失败，告辞。" + err.Error())
		}
		typ := j["type"].(string)
		if typ == "CONNECT" {
			wsId = j["id"].(string)
			writeInit(conn, "TERMINAL_INIT")
			ch <- 0
		} else if typ == "TERMINAL_DATA" {
			content := j["data"].(string)
			// log.Print("Got: ", content)
			for i := 0; i < len(content); i++ {
				recv := content[i]
				if recv != '\r' && recv != '\n' && recv != '!' && recv != ',' && recv != '?' {
					ch <- recv
				}
			}
		}
	}
}

func websocketWriter(ch <-chan uint8, conn *websocket.Conn) {
	bufferCap := 200
	var buf bytes.Buffer
	for {
	recv:
		for {
			select {
			case b := <-ch:
				buf.WriteByte(b)
				if buf.Len() > bufferCap {
					break recv
				}
			case <-time.After(10 * time.Millisecond):
				break recv
			}
		}
		if buf.Len() != 0 {
			writeStdin(conn, buf.String() + "\r")
			buf.Reset()
		}
	}
}

func ping(mux *smux.Session) {
	testStream, err := mux.Open()
	defer testStream.Close()
	if err != nil {
		panic("无法创建测试流，告辞。" + err.Error())
	}

	log.Print("正在测试连通性...")
	testStream.Write([]byte{1}) // ping
	var pingResponse [1]byte
	n, err := testStream.Read(pingResponse[:])
	if err != nil {
		panic(err)
	}
	if n != len(pingResponse[:]) {
		panic("回复的字节数不对。")
	}
	if pingResponse[0] != 55 {
		panic("回复的内容不对。")
	}
	log.Print("连通性测试成功。")
}

func sendExit(mux *smux.Session) {
	testStream, err := mux.Open()
	defer testStream.Close()
	if err != nil {
		panic("无法创建测试流，告辞。" + err.Error())
	}

	testStream.Write([]byte{2}) // exit
	var pingResponse [1]byte
	testStream.Read(pingResponse[:])
}

func setupCloseHandler(mux *smux.Session) {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Print("正在通知退出...")
		sendExit(mux)
		os.Exit(0)
	}()
}

type mappingConfig struct {
	localPort  uint16
	remoteAddr string
	remotePort uint16
}

func handleClient(session *smux.Session, p1 net.Conn, config mappingConfig) {
	logln := func(v ...interface{}) {
		log.Println(v...)
	}
	defer p1.Close()
	p2, err := session.OpenStream()
	if err != nil {
		logln(err)
		return
	}

	remoteEP := fmt.Sprintf("%s:%d", config.remoteAddr, config.remotePort)
	ip := net.ParseIP(config.remoteAddr)
	p2.Write([]byte{0}) // conn
	p2.Write(ip.To4())
	var bs [2]byte
	binary.LittleEndian.PutUint16(bs[:], config.remotePort)
	logln(fmt.Sprintf("远端正在连接 %s...", remoteEP))
	p2.Write(bs[:])

	defer p2.Close()

	var br [1]byte
	p2.Read(br[:])
	if br[0] != 99 {
		logln("无法连接到目标服务器", br[0])
		return
	}

	logln(fmt.Sprintf("连接建立 %s <--> %s (%d)", p1.RemoteAddr(), remoteEP, p2.ID()))
	defer logln(fmt.Sprintf("连接断开 %s <--> %s (%d)", p1.RemoteAddr(), remoteEP, p2.ID()))

	// start tunnel & wait for tunnel termination
	streamCopy := func(dst io.Writer, src io.ReadCloser) {
		if _, err := generic.Copy(dst, src); err != nil {
			// report protocol error
			if err == smux.ErrInvalidProtocol {
				log.Println("smux", err, "in:", p1.RemoteAddr(), "out:", fmt.Sprint(p2.RemoteAddr(), "(", p2.ID(), ")"))
			}
		}
		p1.Close()
		p2.Close()
	}

	go streamCopy(p1, p2)
	streamCopy(p2, p1)
}

func processConfigs(args []string) []mappingConfig {
	result := make([]mappingConfig, len(args))
	for i, elm := range args {
		pm := strings.Split(elm, ":")
		lp, err := strconv.ParseUint(pm[0], 10, 16)
		if err != nil {
			panic("无法转换本地端口号")
		}
		rp, err := strconv.ParseUint(pm[2], 10, 16)
		if err != nil {
			panic("无法转换远端端口号")
		}
		result[i].localPort = uint16(lp)
		result[i].remotePort = uint16(rp)
		result[i].remoteAddr = pm[1]
	}
	return result
}

func listenRemote(mux *smux.Session, cfg mappingConfig) {
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.localPort))
	if err != nil {
		panic(fmt.Sprintf("端口 %d 侦听失败：%s", cfg.localPort, err.Error()))
	}
	log.Println("正在侦听", listener.Addr())

	for {
		p1, err := listener.Accept()
		if err != nil {
			log.Fatalf("%+v", err)
		}

		go handleClient(mux, p1, cfg)
	}
}

func main() {
	rand.Seed(time.Now().UnixNano())

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "请到 https://github.com/karin0/educg-proxy 查看具体用法。\n")
		flag.PrintDefaults()
	}

	if len(os.Args) < 3 {
		flag.Usage()
		return
	}

	var host string
	var cookie string
	var bin_loc string
	var extraQs string

	flag.StringVar(&host, "host", "", "JumpServer 地址")
	flag.StringVar(&cookie, "cookie", "", "JumpServer 网页端的 Cookies")
	flag.StringVar(&bin_loc, "bin", "/home/jovyan/server", "你的 server 文件在服务器上的位置")
	flag.StringVar(&extraQs, "qs", "", "VPN 需要的额外 query string")
	flag.Parse()
	configs := processConfigs(flag.Args())

	c := getWsConn(host, cookie, extraQs)

	downlinkChannel := make(chan uint8, 1024)
	uplinkChannel := make(chan uint8, 1024)

	go websocketReader(downlinkChannel, c)
	_ = <- downlinkChannel  // init message

	log.Print("正在读取多余字符...")
	readGarbage(downlinkChannel, c)

	log.Print("正在同步终端状态...")
	go websocketWriter(uplinkChannel, c)

	writeCmd := func(str string) {
		writeStdin(c, str)
	}

	writeCmd("\rstty -echo\r")
	time.Sleep(2 * time.Second)

	writeCmd(fmt.Sprintf("chmod +x %s\r", bin_loc))
	syncStr := utils.RandStringRunes(16)
	writeCmd(fmt.Sprintf("exec %s %s\r", bin_loc, syncStr))

	// 这个地方应该用 KMP 自动机匹配，但是考虑到同步字符串是随机的，重复的可能性太小，就不用 KMP 了
	syncStr = "QAQ" + syncStr  // to tolerate echo
	syncPos := 0
	for syncPos != len(syncStr) {
		b := <-downlinkChannel
		if b == syncStr[syncPos] {
			syncPos++
		} else {
			syncPos = 0
		}
	}
	log.Print("同步成功！")

	muxStream := CreateConsoleStream(downlinkChannel, uplinkChannel, context.TODO())
	mux, err := smux.Client(muxStream, smux.DefaultConfig())
	if err != nil {
		panic("无法创建 smux 复用器，告辞。" + err.Error())
	}

	ping(mux)
	setupCloseHandler(mux)

	for _, cfg := range configs {
		go listenRemote(mux, cfg)
	}

	select {}
}
