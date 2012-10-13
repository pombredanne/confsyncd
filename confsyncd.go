package main

import (
	"os"
	"log"
	"flag"
	"time"
	"runtime"
	"io/ioutil"
	"encoding/json"
	"path/filepath"
	zmq "github.com/alecthomas/gozmq"
	"github.com/howeyc/fsnotify"
)

type Config struct {
	Time int64
	Body string
}

func readConfig(filename string) Config {
	file, _ := os.Open(filename)
	defer file.Close()
	content, _ := ioutil.ReadAll(file)
	stat, _ := file.Stat()
	return Config{stat.ModTime().UnixNano(), string(content)}
}

func writeConfig(filename string, content []byte) {
	err := ioutil.WriteFile(filename, content, 0x777)
	if err != nil {
		log.Fatal(err)
	}
}

func publishConfig(config Config, global_pub_socket, local_pub_socket zmq.Socket) {
	m, _ := json.Marshal(config)
	global_pub_socket.Send(m, 0)
	local_pub_socket.Send([]byte(config.Body), 0)
}

func watchConfig(filename string, global_pub_socket, local_pub_socket zmq.Socket) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	err = watcher.Watch(filename)
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case _ = <-watcher.Event:
			local_config := readConfig(filename)
			publishConfig(local_config, global_pub_socket, local_pub_socket)
		case err := <-watcher.Error:
			log.Println("Watcher error:", err)
		}
		runtime.Gosched()
	}
}

func watchSub(global_sub_socket, global_pub_socket, local_pub_socket zmq.Socket, filename string) {
	for {
		data, _ := global_sub_socket.Recv(0)
		var msg Config
		_ = json.Unmarshal(data, &msg)
		local_config := readConfig(filename)
		if msg.Time > local_config.Time {
			body := []byte(msg.Body)
			writeConfig(filename, body)
			local_pub_socket.Send(body, 0)
		} else if msg.Time < local_config.Time {
			m, _ := json.Marshal(local_config)
			global_pub_socket.Send(m, 0)
		}
		runtime.Gosched()
	}
}

func openSocket(ctx zmq.Context, t zmq.SocketType, url string) zmq.Socket {
	socket, _ := ctx.NewSocket(t)
	if t == zmq.PUB {
		socket.Bind(url)
	} else {
		socket.Connect(url)
	}
	return socket
}

var sub_adr = flag.String("sub", "", "pub socket of a confsyncd to connect")
var pub_port = flag.String("pub", "5556", "pub port")
var conf_filepath = flag.String("file", "config.json", "config file to sync")

func main() {
	flag.Parse()
	filename, _ := filepath.Abs(*conf_filepath)
	ctx, _ := zmq.NewContext()
	defer ctx.Close()

	global_pub_socket := openSocket(ctx, zmq.PUB, "tcp://*:"+*pub_port)
	defer global_pub_socket.Close()
	local_pub_socket  := openSocket(ctx, zmq.PUB, "ipc://confsyncd")
	defer local_pub_socket.Close()
	global_sub_socket, _ := ctx.NewSocket(zmq.SUB)
	global_sub_socket.SetSockOptString(zmq.SUBSCRIBE, "")
	if *sub_adr != "" {
		global_sub_socket.Connect(*sub_adr)
	}
	defer global_sub_socket.Close()

	go watchSub(global_sub_socket, global_pub_socket, local_pub_socket, filename)

	time.Sleep(1e9/2)
	local_config := readConfig(filename)
	publishConfig(local_config, global_pub_socket, local_pub_socket)

	watchConfig(filename, global_pub_socket, local_pub_socket)
}
