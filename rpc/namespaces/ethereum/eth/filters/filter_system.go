// Copyright 2021 Evmos Foundation
// This file is part of Evmos' Ethermint library.
//
// The Ethermint library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The Ethermint library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the Ethermint library. If not, see https://github.com/novic-labs/novic/v2/blob/main/LICENSE
package filters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/cometbft/cometbft/libs/log"
	tmpubsub "github.com/cometbft/cometbft/libs/pubsub"
	tmquery "github.com/cometbft/cometbft/libs/pubsub/query"
	rpcclient "github.com/cometbft/cometbft/rpc/client"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/filters"
	"github.com/ethereum/go-ethereum/rpc"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/novic-labs/novic/v2/rpc/ethereum/pubsub"
	evmtypes "github.com/novic-labs/novic/v2/x/evm/types"
)

const (
	subscriberName     = "eth_filter"
	subscribBufferSize = 2048
)

var (
	txEvents  = tmtypes.QueryForEvent(tmtypes.EventTx).String()
	evmEvents = tmquery.MustParse(fmt.Sprintf("%s='%s' AND %s.%s='%s'",
		tmtypes.EventTypeKey,
		tmtypes.EventTx,
		sdk.EventTypeMessage,
		sdk.AttributeKeyModule, evmtypes.ModuleName)).String()
	headerEvents = tmtypes.QueryForEvent(tmtypes.EventNewBlockHeader).String()
)

// EventSystem creates subscriptions, processes events and broadcasts them to the
// subscription which match the subscription criteria using the Tendermint's RPC client.
type EventSystem struct {
	logger     log.Logger
	ctx        context.Context
	tmWSClient rpcclient.EventsClient

	// light client mode
	lightMode bool

	index      filterIndex
	topicChans map[string]chan<- coretypes.ResultEvent
	indexMux   *sync.RWMutex

	// Channels
	install   chan *Subscription // install filter for event notification
	uninstall chan *Subscription // remove filter for event notification
	eventBus  pubsub.EventBus
}

// NewEventSystem creates a new manager that listens for event on the given mux,
// parses and filters them. It uses the all map to retrieve filter changes. The
// work loop holds its own index that is used to forward events to filters.
//
// The returned manager has a loop that needs to be stopped with the Stop function
// or by stopping the given mux.
func NewEventSystem(logger log.Logger, tmWSClient rpcclient.EventsClient) *EventSystem {
	index := make(filterIndex)
	for i := filters.UnknownSubscription; i < filters.LastIndexSubscription; i++ {
		index[i] = make(map[rpc.ID]*Subscription)
	}

	es := &EventSystem{
		logger:     logger,
		ctx:        context.Background(),
		tmWSClient: tmWSClient,
		lightMode:  false,
		index:      index,
		topicChans: make(map[string]chan<- coretypes.ResultEvent, len(index)),
		indexMux:   new(sync.RWMutex),
		uninstall:  make(chan *Subscription),
		eventBus:   pubsub.NewEventBus(),
	}

	go es.eventLoop()
	return es
}

// WithContext sets a new context to the EventSystem. This is required to set a timeout context when
// a new filter is intantiated.
func (es *EventSystem) WithContext(ctx context.Context) {
	es.ctx = ctx
}

// subscribe performs a new event subscription to a given Tendermint event.
// The subscription creates a unidirectional receive event channel to receive the ResultEvent.
func (es *EventSystem) subscribe(sub *Subscription) (*Subscription, pubsub.UnsubscribeFunc, error) {
	var (
		err      error
		cancelFn context.CancelFunc
		chEvents <-chan coretypes.ResultEvent
	)

	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	existingSubs := es.eventBus.Topics()
	for _, topic := range existingSubs {
		if topic == sub.event {
			eventCh, unsubFn, err := es.eventBus.Subscribe(sub.event)
			if err != nil {
				err := errors.Wrapf(err, "failed to subscribe to topic: %s", sub.event)
				return nil, nil, err
			}

			sub.eventCh = eventCh
			return sub, unsubFn, nil
		}
	}

	switch sub.typ {
	case filters.LogsSubscription:
		chEvents, err = es.tmWSClient.Subscribe(ctx, subscriberName, sub.event, subscribBufferSize)
	case filters.BlocksSubscription:
		chEvents, err = es.tmWSClient.Subscribe(ctx, subscriberName, sub.event, subscribBufferSize)
	case filters.PendingTransactionsSubscription:
		chEvents, err = es.tmWSClient.Subscribe(ctx, subscriberName, sub.event, subscribBufferSize)
	default:
		err = fmt.Errorf("invalid filter subscription type %d", sub.typ)
	}

	if err != nil && !errors.Is(err, tmpubsub.ErrAlreadySubscribed) {
		sub.err <- err
		return nil, nil, err
	}

	if err := es.eventBus.AddTopic(sub.event, chEvents); err != nil {
		return nil, nil, err
	}

	eventCh, unsubFn, err := es.eventBus.Subscribe(sub.event)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to subscribe to topic after installed: %s", sub.event)
	}

	sub.eventCh = eventCh
	return sub, unsubFn, nil
}

// SubscribeLogs creates a subscription that will write all logs matching the
// given criteria to the given logs channel. Default value for the from and to
// block is "latest". If the fromBlock > toBlock an error is returned.
func (es *EventSystem) SubscribeLogs(crit filters.FilterCriteria) (*Subscription, pubsub.UnsubscribeFunc, error) {
	var from, to rpc.BlockNumber
	if crit.FromBlock == nil {
		from = rpc.LatestBlockNumber
	} else {
		from = rpc.BlockNumber(crit.FromBlock.Int64())
	}
	if crit.ToBlock == nil {
		to = rpc.LatestBlockNumber
	} else {
		to = rpc.BlockNumber(crit.ToBlock.Int64())
	}

	switch {
	// only interested in new mined logs, mined logs within a specific block range, or
	// logs from a specific block number to new mined blocks
	case (from == rpc.LatestBlockNumber && to == rpc.LatestBlockNumber),
		(from >= 0 && to >= 0 && to >= from),
		(from >= 0 && to == rpc.LatestBlockNumber):
		return es.subscribeLogs(crit)

	default:
		return nil, nil, fmt.Errorf("invalid from and to block combination: from > to (%d > %d)", from, to)
	}
}

// subscribeLogs creates a subscription that will write all logs matching the
// given criteria to the given logs channel.
func (es *EventSystem) subscribeLogs(crit filters.FilterCriteria) (*Subscription, pubsub.UnsubscribeFunc, error) {
	sub := &Subscription{
		id:       rpc.NewID(),
		typ:      filters.LogsSubscription,
		event:    evmEvents,
		logsCrit: crit,
		created:  time.Now().UTC(),
		logs:     make(chan []*ethtypes.Log),
		err:      make(chan error, 1),
	}
	return es.subscribe(sub)
}

// SubscribeNewHeads subscribes to new block headers events.
func (es EventSystem) SubscribeNewHeads() (*Subscription, pubsub.UnsubscribeFunc, error) {
	sub := &Subscription{
		id:      rpc.NewID(),
		typ:     filters.BlocksSubscription,
		event:   headerEvents,
		created: time.Now().UTC(),
		headers: make(chan *ethtypes.Header),
		err:     make(chan error, 1),
	}
	return es.subscribe(sub)
}

// SubscribePendingTxs subscribes to new pending transactions events from the mempool.
func (es EventSystem) SubscribePendingTxs() (*Subscription, pubsub.UnsubscribeFunc, error) {
	sub := &Subscription{
		id:      rpc.NewID(),
		typ:     filters.PendingTransactionsSubscription,
		event:   txEvents,
		created: time.Now().UTC(),
		hashes:  make(chan []common.Hash),
		err:     make(chan error, 1),
	}
	return es.subscribe(sub)
}

type filterIndex map[filters.Type]map[rpc.ID]*Subscription

// eventLoop (un)installs filters and processes mux events.
func (es *EventSystem) eventLoop() {
	for f := range es.uninstall {
		es.indexMux.Lock()
		delete(es.index[f.typ], f.id)

		var channelInUse bool
		for _, sub := range es.index[f.typ] {
			if sub.event == f.event {
				channelInUse = true
				break
			}
		}
		// remove topic only when channel is not used by other subscriptions
		if !channelInUse {
			if err := es.tmWSClient.Unsubscribe(es.ctx, subscriberName, f.event); err != nil {
				es.logger.Error("failed to unsubscribe from query", "query", f.event, "error", err.Error())
			}

			// gracefully handle lagging subscribers
			ch, ok := es.topicChans[f.event]
			if ok {
				es.eventBus.RemoveTopic(f.event)
				close(ch)
				delete(es.topicChans, f.event)
			}
		}

		es.indexMux.Unlock()
		close(f.err)
	}
}
