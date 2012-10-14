package main

import (
	"os"
	"log"
	"net"
	"flag"
	"time"
	"runtime"
	"strconv"
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

type Request struct {
	Type string
}

type ConnRequest struct {
	Type string
	PubAddress string
	RepAddress string
}

type ConnReply struct {
	PubAddress string
	Clients []string
}

// Config actions {{{
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
// }}}

// Event handlers {{{
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

func watchRep(ctx zmq.Context, global_rep_socket, global_sub_socket zmq.Socket, clients []string, pub_address string) {
	for {
		data, _ := global_rep_socket.Recv(0)
		var req Request
		_ = json.Unmarshal(data, &req)
		if req.Type == "connect" {
			var creq ConnRequest
			json.Unmarshal(data, &creq)
			global_sub_socket.Connect(creq.PubAddress)
			reply, _ := json.Marshal(ConnReply{pub_address, clients})
			global_rep_socket.Send(reply, 0)
			clients = append(clients, creq.RepAddress)
		}
		runtime.Gosched()
	}
}
// }}}

// Helper functions {{{
func openSocket(ctx zmq.Context, t zmq.SocketType, url string) zmq.Socket {
	socket, _ := ctx.NewSocket(t)
	if t == zmq.PUB || t == zmq.REP {
		socket.Bind(url)
	} else {
		socket.Connect(url)
	}
	return socket
}

func findOpenPort() int {
	for i := 1024; i < 65536; i++ {
		_, err := net.Dial("tcp", "localhost:"+strconv.Itoa(i))
		if err != nil {
			return i
		}
	}
	return 5555 // will never return this unless you use up all the ports
}
// }}}

var conn_adr = flag.String("connect", "", "address of a confsyncd instance to connect")
var conf_filepath = flag.String("file", "config.json", "config file to sync")
var rep_port = flag.Int("port", 0, "global rep socket port, default is random")

func main() {
	flag.Parse()
	filename, _ := filepath.Abs(*conf_filepath)
	ctx, _ := zmq.NewContext()
	defer ctx.Close()

	global_pub_port := strconv.Itoa(findOpenPort())
	global_pub_socket := openSocket(ctx, zmq.PUB, "tcp://*:"+global_pub_port)
	pub_address := "tcp://localhost:"+global_pub_port // TODO: not just localhost
	defer global_pub_socket.Close()

	local_pub_socket  := openSocket(ctx, zmq.PUB, "ipc://confsyncd")
	defer local_pub_socket.Close()

	global_sub_socket, _ := ctx.NewSocket(zmq.SUB)
	global_sub_socket.SetSockOptString(zmq.SUBSCRIBE, "")
	defer global_sub_socket.Close()

	var global_rep_port string
	if *rep_port == 0 {
		global_rep_port = strconv.Itoa(findOpenPort())
	} else {
		global_rep_port = strconv.Itoa(*rep_port)
	}
	global_rep_socket := openSocket(ctx, zmq.REP, "tcp://*:"+global_rep_port)
	defer global_rep_socket.Close()
	log.Printf("Address: tcp://localhost:"+global_rep_port)

	clients := make([]string, 0)

	time.Sleep(1e9/2)
	local_config := readConfig(filename)
	publishConfig(local_config, global_pub_socket, local_pub_socket)

	if *conn_adr != "" {
		req_socket := openSocket(ctx, zmq.REQ, *conn_adr)
		conn_req := ConnRequest{"connect", pub_address, "tcp://localhost:"+global_rep_port}
		conn_req_json, _ := json.Marshal(conn_req)
		req_socket.Send(conn_req_json, 0)
		reply, _ := req_socket.Recv(0)
		var conn_reply ConnReply
		json.Unmarshal(reply, &conn_reply)
		global_sub_socket.Connect(conn_reply.PubAddress)
		copy(clients, conn_reply.Clients)
		req_socket.Close()
	}

	go watchRep(ctx, global_rep_socket, global_sub_socket, clients, pub_address)
	go watchSub(global_sub_socket, global_pub_socket, local_pub_socket, filename)
	watchConfig(filename, global_pub_socket, local_pub_socket)
}
