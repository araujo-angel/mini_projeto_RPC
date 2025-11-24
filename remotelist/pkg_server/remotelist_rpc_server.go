package main

import (
	"fmt"
	"ifpb/remotelist/pkg_structs"
	"net"
	"net/rpc"
)

func main() {
	list := remotelist.NewRemoteList()
	rpcs := rpc.NewServer()
	rpcs.Register(list)
	l, e := net.Listen("tcp", "[localhost]:5000")
	defer l.Close()
	if e != nil {
		fmt.Println("listen error:", e)
	}
	for {
		conn, err := l.Accept()
		if err == nil {
			go rpcs.ServeConn(conn) //goroutines permite multiplos clientes se conectarem
		} else {
			break
		}
	}
}
