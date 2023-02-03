package demo
//简单实现一个websocket的服务端的核心代码
import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/gobwas/ws"
	"github.com/sirupsen/logrus"
)

type Server struct {
	once sync.Once
	sync.Mutex
	id      string
	address string
	users   map[string]net.Conn
}

func NewServer(id, address string) *Server {
	return newserver(id, address)
}
func newserver(id, address string) *Server {
	return &Server{
		id:      id,
		address: address,
		users:   make(map[string]net.Conn, 100),
	}
}
func (s *Server) Start() error {
	mux := http.NewServeMux()
	log := logrus.WithFields(logrus.Fields{
		"module": "Server",
		"listen": s.address,
		"id":     s.id,
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		//step1 upgrade http using websocket tools
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			conn.Close()
			return
		}
		//step2 get user id from url(query)
		user := r.URL.Query().Get("user")
		if user == "" {
			conn.Close()
			return
		}
		//step3 add the user form query to the user-map of the server
		old, ok := s.addUser(user, conn)
		//got to close the former connetion so that the new connection can work
		if ok {
			old.Close()
		}
		log.Infof("user %s in", user)

		go func(user string, conn net.Conn) {
			err := s.readloop(user, conn)
			if err != nil {
				log.Error(err)
			}
			conn.Close()
			s.delUser(user)
			log.Infof("connection of %s closed", user)
		}(user, conn)
	})
	log.Infoln("started")
	return http.ListenAndServe(s.address, mux)
}
func (s *Server) addUser(user string, conn net.Conn) (net.Conn, bool) {
	s.Lock()
	defer s.Unlock()
	old, ok := s.users[user]
	s.users[user] = conn
	return old, ok
}
//read the message that the user send 
func (s *Server) readloop(user string, conn net.Conn) error {
	for{
		frame,err:=ws.ReadFrame(conn)
		if err!=nil{
			return err
		}
		if frame.Header.OpCode==ws.OpClose{
			return errors.New("remoute side close the connection")
		}
		if frame.Header.Masked{
			ws.Cipher(frame.Payload,frame.Header.Mask,0)
		}
		if frame.Header.OpCode==ws.OpText{
			go s.handle(user,string(frame.Payload))
		}
	}
}
func (s *Server) delUser(user string){
	s.Lock()
	defer s.Unlock()
	delete(s.users, user)
}
func (s *Server) Shutdown() {
	s.once.Do(func() {
		s.Lock()
		defer s.Unlock()
		for _, conn := range s.users {
			conn.Close()
		}
	})
}
func (s*Server)handle(user string ,message string){
	logrus.Infof("recieve message  %s from %s",message,user)
	s.Lock()
	defer s.Unlock()
	broadcast:=fmt.Sprintf("%s -- FROM %s",message,user)
	for u,conn:=range s.users{
		if u==user{
			continue
		}
		err:=s.writeText(conn,broadcast)
		if err!=nil{
			logrus.Errorf("write to %s failed,error : %v",user,err)
		}
		logrus.Infof("send to %s : %s",u,broadcast)
	}
}
func (s*Server)writeText(conn net.Conn,message string)error{
	f:=ws.NewTextFrame([]byte(message))
	return ws.WriteFrame(conn,f)
}
