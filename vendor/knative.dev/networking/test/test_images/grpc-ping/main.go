/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"

	networkingpkg "knative.dev/networking/pkg"
	ping "knative.dev/networking/test/test_images/grpc-ping/proto"
	"knative.dev/pkg/network"
)

var (
	delay    int64
	hostname string
)

func pong(req *ping.Request) *ping.Response {
	return &ping.Response{Msg: req.Msg + hostname + os.Getenv("SUFFIX")}
}

type server struct{}

func (s *server) Ping(ctx context.Context, req *ping.Request) (*ping.Response, error) {
	log.Printf("Received ping: %v", req.Msg)

	time.Sleep(time.Duration(delay) * time.Millisecond)

	resp := pong(req)

	log.Printf("Sending pong: %v", resp.Msg)
	return resp, nil
}

func (s *server) PingStream(stream ping.PingService_PingStreamServer) error {
	log.Printf("Starting stream")
	for {
		req, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			log.Printf("Ending stream")
			return nil
		}
		if err != nil {
			log.Print("Failed to receive ping: ", err)
			return err
		}

		log.Printf("Received ping: %v", req.Msg)

		resp := pong(req)

		log.Printf("Sending pong: %v", resp.Msg)
		err = stream.Send(resp)
		if err != nil {
			log.Print("Failed to send pong: ", err)
			return err
		}
	}
}

func httpWrapper(g *grpc.Server) http.Handler {
	return networkingpkg.NewProbeHandler(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ProtoMajor == 2 && r.Header.Get("Content-Type") == "application/grpc" {
				g.ServeHTTP(w, r)
			}
		}),
	)
}

func main() {
	log.Printf("Starting server on %s", os.Getenv("PORT"))

	delay, _ = strconv.ParseInt(os.Getenv("DELAY"), 10, 64)
	log.Printf("Using DELAY of %d ms", delay)

	if wantHostname, _ := strconv.ParseBool(os.Getenv("HOSTNAME")); wantHostname {
		hostname, _ = os.Hostname()
		log.Print("Setting hostname in response ", hostname)
	}

	g := grpc.NewServer()
	s := network.NewServer(":"+os.Getenv("PORT"), httpWrapper(g))

	ping.RegisterPingServiceServer(g, &server{})

	if cert, key := os.Getenv("CERT"), os.Getenv("KEY"); cert != "" && key != "" {
		log.Fatal(s.ListenAndServeTLS(cert, key))
	} else {
		log.Fatal(s.ListenAndServe())
	}
}
