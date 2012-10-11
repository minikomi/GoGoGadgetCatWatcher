package main

import (
	"bufio"
	"code.google.com/p/go.net/websocket"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
)

var (
	frmt string = "2006-01-02 15:04:05.000"
)

type AdbLog struct {
	Time     int64
	Tag      string
	Message  string
	Priority string
	PID      string
	TID      string
}

func CloseOnSignal(c <-chan os.Signal, cmd *exec.Cmd) {
	sig := <-c
	log.Println("os.Signal Received:", sig, "Killing process.")
	cmd.Process.Kill()
	return
}

func ParseAdbLineToJSON(str string) (jadblog []byte, err error) {

	splitString := strings.Split(str, ": ")

	if len(splitString) > 1 {
		meta := strings.Fields(splitString[0])
		timestring := fmt.Sprintf("%d-%s %s", time.Now().Year(), meta[0], meta[1])

		timeCode, err := time.Parse(frmt, timestring)
		if err != nil {
			log.Println("Time Parse Error:", err)
			return jadblog, err
		}

		adblog := AdbLog{timeCode.Unix(), meta[5], splitString[1], meta[4], meta[2], meta[3]}

		jadblog, err = json.Marshal(adblog)
		if err != nil {
			log.Println("JSON Error:", err)
			return jadblog, err
		}
	}
	return jadblog, nil
}

func IndexServer(w http.ResponseWriter, req *http.Request) {
	log.Println("Path:", req.URL.Path)
	if req.URL.Path == "/" {
		http.ServeFile(w, req, "index.html")
	} else {
		http.ServeFile(w, req, "."+req.URL.Path)
	}
}

type Hub struct {
	Connections map[*Socket]bool
	Pipe        chan string
}

type Socket struct {
	Ws *websocket.Conn
}

func EchoServer(ws *websocket.Conn) {
	s := &Socket{ws}
	h.Connections[s] = true
	s.ReceiveMessage()
}

var h Hub

func (h *Hub) Broadcast() {
	for {
		var x string
		x = <-h.Pipe
		for s, _ := range h.Connections {
			err := websocket.Message.Send(s.Ws, x)
			if err != nil {
				delete(h.Connections, s)
			}
		}
	}
}

func (s *Socket) ReceiveMessage() {
	websocket.Message.Send(s.Ws, "Welcome")
	for {
		var x string
		err := websocket.Message.Receive(s.Ws, &x)
		if err != nil {
			break
		}
		h.Pipe <- x
	}
	s.Ws.Close()
}

func main() {
	cmd := exec.Command("adb", "logcat", "-v", "threadtime")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Fatal("Pipe Error:", err)
	}

	defer stdout.Close()

	rd := bufio.NewReader(stdout)
	if err := cmd.Start(); err != nil {
		log.Fatal("Buffer Error:", err)
	}

	//Make sure child process exits on interrupt / kill
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, os.Kill)
	go CloseOnSignal(c, cmd)

	h.Connections = make(map[*Socket]bool)
	h.Pipe = make(chan string, 1)
	go h.Broadcast()

	go (func() {

		for {
			str, err := rd.ReadString('\n')
			if err != nil {
				log.Fatal("Read Error:", err)
				return
			}

			jadblog, err := ParseAdbLineToJSON(str)
			if err != nil {
				continue
			}
			h.Pipe <- (string(jadblog))
		}
	})()

	http.Handle("/ws", websocket.Handler(EchoServer))
	http.HandleFunc("/", IndexServer)

	err = http.ListenAndServe(":12345", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
