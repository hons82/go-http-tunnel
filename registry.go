// Copyright (C) 2017 Michał Matczuk
// Use of this source code is governed by an AGPL-style
// license that can be found in the LICENSE file.

package tunnel

import (
	"fmt"
	"net"
	"sync"

	"github.com/hons82/go-http-tunnel/id"
	"github.com/hons82/go-http-tunnel/log"
)

// RegistryItem holds information about hosts and listeners associated with a
// client.
type RegistryItem struct {
	*id.IDInfo
	Hosts     []*HostAuth
	Listeners []net.Listener
}

// HostAuth holds host and authentication info.
type HostAuth struct {
	Host string
	Auth *Auth
}

type hostInfo struct {
	*id.IDInfo
	identifier id.ID
	auth       *Auth
}

type registry struct {
	items  map[id.ID]*RegistryItem
	hosts  map[string]*hostInfo
	mu     sync.RWMutex
	logger log.Logger
}

func newRegistry(logger log.Logger) *registry {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	return &registry{
		items:  make(map[id.ID]*RegistryItem),
		hosts:  make(map[string]*hostInfo),
		logger: logger,
	}
}

var voidRegistryItem = &RegistryItem{}

// Subscribe allows to connect client with a given identifier.
func (r *registry) Subscribe(identifier id.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[identifier]; ok {
		return
	}

	r.logger.Log(
		"level", 2,
		"action", "subscribe",
		"identifier", identifier,
	)

	r.items[identifier] = voidRegistryItem
}

// IsSubscribed returns true if client is subscribed.
func (r *registry) IsSubscribed(identifier id.ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.items[identifier]
	return ok
}

// Subscriber returns client identifier assigned to given host.
func (r *registry) Subscriber(hostPort string) (id.ID, *Auth, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hosts[trimPort(hostPort)]
	if !ok {
		return id.ID{}, nil, false
	}

	return h.identifier, h.auth, ok
}

func (r *registry) HasTunnel(hostPort string, identifier id.ID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	h, ok := r.hosts[trimPort(hostPort)]

	return ok && h.identifier.Equals(identifier)
}

// Unsubscribe removes client from registry and returns it's RegistryItem.
func (r *registry) Unsubscribe(identifier id.ID, autoSubscribe bool) *RegistryItem {
	r.mu.Lock()
	defer r.mu.Unlock()

	i, ok := r.items[identifier]
	if !ok || (autoSubscribe && i == voidRegistryItem) {
		return nil
	}

	r.logger.Log(
		"level", 2,
		"action", "unsubscribe",
		"identifier", identifier,
	)

	if autoSubscribe {
		if i.Hosts != nil {
			for _, h := range i.Hosts {
				delete(r.hosts, h.Host)
			}
		}
		delete(r.items, identifier)
	} else {
		r.items[identifier] = voidRegistryItem
	}

	return i
}

func (r *registry) set(i *RegistryItem, identifier id.ID) error {
	r.logger.Log(
		"level", 2,
		"action", "set registry item",
		"identifier", identifier,
	)

	r.mu.Lock()
	defer r.mu.Unlock()

	j, ok := r.items[identifier]
	if !ok {
		return errClientNotSubscribed
	}
	if j != voidRegistryItem {
		return fmt.Errorf("attempt to overwrite registry item")
	}

	if i.Hosts != nil {
		for _, h := range i.Hosts {
			if h.Auth != nil && h.Auth.User == "" {
				return fmt.Errorf("missing auth user")
			}
			if hi, ok := r.hosts[trimPort(h.Host)]; ok && !hi.identifier.Equals(identifier) {
				return fmt.Errorf("host %q is occupied", h.Host)
			}
		}

		for _, h := range i.Hosts {
			r.hosts[trimPort(h.Host)] = &hostInfo{
				identifier: identifier,
				auth:       h.Auth,
			}
		}
	}

	r.items[identifier] = i

	return nil
}

func (r *registry) registerTunnel(host string, client string) error {
	identifier := id.New([]byte(client))

	r.logger.Log(
		"level", 3,
		"action", "register tunnel",
		"host", host,
		"client", client,
		"identifier", identifier,
	)

	r.Subscribe(identifier)

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.hosts[trimPort(host)]; ok {
		return fmt.Errorf("host %q is occupied", host)
	}

	r.hosts[trimPort(host)] = &hostInfo{
		identifier: identifier,
		IDInfo: &id.IDInfo{
			Client: client,
		},
	}

	return nil
}

// Clear removes all items from the registry
func (r *registry) Clear() {
	r.logger.Log(
		"level", 3,
		"action", "clear registry ",
	)

	r.mu.Lock()
	defer r.mu.Unlock()

	for k := range r.hosts {
		delete(r.hosts, k)
	}

	for i := range r.items {
		delete(r.items, i)
	}
}

func trimPort(hostPort string) (host string) {
	host, _, _ = net.SplitHostPort(hostPort)
	if host == "" {
		host = hostPort
	}
	return
}
