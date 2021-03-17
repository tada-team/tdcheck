package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"gopkg.in/yaml.v2"
)

func main() {
	configPathPtr := flag.String("config", "/etc/tdcheck/default.yml", "path to config")
	flag.Parse()

	b, err := ioutil.ReadFile(*configPathPtr)
	if err != nil {
		log.Println("config error:", err)
		os.Exit(1)
	}

	var config struct {
		Listen  string   `yaml:"listen"`
		Servers []Server `yaml:"servers"`
	}

	if err := yaml.Unmarshal(b, &config); err != nil {
		log.Println("config error:", err)
		os.Exit(1)
	}

	rtr := mux.NewRouter()
	for _, s := range config.Servers {
		ServerWatch(s, rtr)
	}

	srv := http.NewServeMux()
	srv.Handle("/", rtr)

	server := &http.Server{
		Addr:    config.Listen,
		Handler: srv,
	}

	if server.Addr == "" {
		server.Addr = "127.0.0.1:8000"
	}

	log.Printf("start tdcheck at: http://%s", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		log.Println("start server fail:", err)
		os.Exit(1)
	}
}
