package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

var (
	flagWebhookUrl string
)

type WebhookRequest struct {
	Text string `json:"text"`
}

func handleEvent(e events.Message) {
	log.Printf("action:%s %v", e.Action, e.Actor.Attributes)

	name, ok := e.Actor.Attributes["name"]
	if !ok {
		return
	}
	action := e.Action
	switch {
	case action == "start":
		action = "is starting"
	case action == "die":
		action = "is die"
	case strings.Contains(action, "exec_start"):
		action = "is exec-ing " + strings.TrimSpace(action[12:len(action)])
	default:
		log.Printf("unsupported action: %+v", e)
		return
	}

	tmpl := "*%s* is %s:\n```\n%v\n```"
	buf, _ := json.Marshal(WebhookRequest{
		Text: fmt.Sprintf(tmpl, name, action, e.Actor.Attributes),
	})

	resp, err := http.Post(flagWebhookUrl, "application/json", bytes.NewBuffer(buf))
	if err != nil {
		log.Printf("error submitting webhook: %s", err)
		return
	}
	log.Printf("sent webhook with status %d", resp.StatusCode)
}

func main() {
	flag.StringVar(&flagWebhookUrl, "url", "", "webhook url")
	flag.Parse()

	if flagWebhookUrl == "" {
		flag.Usage()
		return
	}

	cli, err := client.NewEnvClient()
	if err != nil {
		log.Fatalf("error creating client: %s", err)
	}
	info, err := cli.Info(context.Background())
	if err != nil {
		log.Fatalf("error fetching server info: %s", err)
	}
	log.Printf("connected to docker api: %s", info.Name)

	filter := filters.NewArgs()
	filter.Add("type", "container")
	filter.Add("event", "start")
	filter.Add("event", "die")
	filter.Add("event", "exec_start")

	msgChan, errChan := cli.Events(context.Background(), types.EventsOptions{
		Filters: filter,
	})
	log.Printf("listening for events")

	for {
		select {
		case err := <-errChan:
			log.Fatalf("error reading events: %s", err)
		case e := <-msgChan:
			handleEvent(e)
		}
	}
}