package engine

import (
	"fmt"

	"github.com/nicholaskh/golib/cache"
	cmap "github.com/nicholaskh/golib/concurrent/map"
	"github.com/nicholaskh/golib/set"
	log "github.com/nicholaskh/log4go"
	"github.com/nicholaskh/pushd/client"
)

var (
	PubsubChannels *PubsubChans = NewPubsubChannels()
)

type PubsubChans struct {
	*cache.LruCache
}

func NewPubsubChannels() (this *PubsubChans) {
	this = new(PubsubChans)
	this.LruCache = cache.NewLruCache(200000)
	return
}

func (this *PubsubChans) Get(channel string) (clients cmap.ConcurrentMap, exists bool) {
	clientsInterface, exists := PubsubChannels.LruCache.Get(channel)
	clients, _ = clientsInterface.(cmap.ConcurrentMap)
	return
}

// TODO subscribe count of channel
func subscribe(cli *client.Client, channel string) string {
	log.Debug("%x", channel)
	_, exists := cli.Channels[channel]
	if exists {
		return fmt.Sprintf("%s %s", OUTPUT_ALREADY_SUBSCRIBED, channel)
	} else {
		cli.Channels[channel] = 1
		clients, exists := PubsubChannels.Get(channel)
		if exists {
			clients.Set(cli.RemoteAddr().String(), cli)
		} else {
			clients = cmap.New()
			clients.Set(cli.RemoteAddr().String(), cli)

			//s2s
			Proxy.SubMsgChan <- channel
		}
		PubsubChannels.Set(channel, clients)

		return fmt.Sprintf("%s %s", OUTPUT_SUBSCRIBED, channel)
	}

}

func unsubscribe(cli *client.Client, channel string) string {
	_, exists := cli.Channels[channel]
	if exists {
		delete(cli.Channels, channel)
		clients, exists := PubsubChannels.Get(channel)
		if exists {
			clients.Remove(cli.RemoteAddr().String())
		}
		clients, exists = PubsubChannels.Get(channel)
		if len(clients) == 0 {
			PubsubChannels.Del(channel)
		}
		return fmt.Sprintf("%s %s", OUTPUT_UNSUBSCRIBED, channel)
	} else {
		return fmt.Sprintf("%s %s", OUTPUT_NOT_SUBSCRIBED, channel)
	}
}

func UnsubscribeAllChannels(cli *client.Client) {
	for channel, _ := range cli.Channels {
		clients, _ := PubsubChannels.Get(channel)
		clients.Remove(cli.RemoteAddr().String())
		if len(clients) == 0 {
			PubsubChannels.Del(channel)
		}
	}
	cli.Channels = nil
}

func publish(channel, msg string, fromS2s bool) string {
	clients, exists := PubsubChannels.Get(channel)
	if exists {
		log.Debug("channel %s subscribed by clients%s", channel, clients)
		for ele := range clients.Iter() {
			cli := ele.Val.(*client.Client)
			cli.Mutex.Acquire()
			if !cli.Closed {
				cli.MsgQueue <- msg
			}
			cli.Mutex.Release()
		}
	}

	if !fromS2s {
		//s2s
		var peers set.Set
		peers, exists = Proxy.GetPeersByChannel(channel)
		log.Debug("now peers %s", peers)
		if exists {
			Proxy.PubMsgChan <- NewPubTuple(peers, msg, channel)
		}

		return OUTPUT_PUBLISHED
	} else {
		return ""
	}
}
