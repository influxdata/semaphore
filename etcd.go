// Copyright 2015 CoreOS, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package semaphore

import (
	"encoding/json"
	"errors"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

// KeysAPI is the minimum etcd client.KeysAPI interface EtcdLockClient needs
// to do its job.
type KeysAPI interface {
	Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error)
	Set(ctx context.Context, key, value string, opts *client.SetOptions) (*client.Response, error)
	Create(ctx context.Context, key, value string) (*client.Response, error)
}

// EtcdLockClient is a wrapper around the etcd client that provides
// simple primitives to operate on the internal semaphore and holders
// structs through etcd.
type EtcdLockClient struct {
	keyapi  KeysAPI
	keypath string
}

type EtcdWrapper struct {
	Etcd client.KeysAPI
}

func (e *EtcdWrapper) Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error) {
	return e.Etcd.Get(ctx.(context.Context), key, opts)
}
func (e *EtcdWrapper) Set(ctx context.Context, key, value string, opts *client.SetOptions) (*client.Response, error) {
	return e.Etcd.Set(ctx.(context.Context), key, value, opts)
}

func (e *EtcdWrapper) Create(ctx context.Context, key, value string) (*client.Response, error) {
	return e.Etcd.Create(ctx.(context.Context), key, value)
}

func NewEtcdWrapper(etcd client.KeysAPI) *EtcdWrapper {
	return &EtcdWrapper{
		Etcd: etcd,
	}
}

// NewEtcdLockClient creates a new EtcdLockClient. The key parameter defines
// the etcd key path in which the client will manipulate the semaphore.
func NewEtcdLockClient(ctx context.Context, keyapi KeysAPI, key string) (*EtcdLockClient, error) {
	elc := &EtcdLockClient{keyapi, key}
	if err := elc.Init(ctx); err != nil {
		return nil, err
	}

	return elc, nil
}

// Init sets an initial copy of the semaphore if it doesn't exist yet.
func (c *EtcdLockClient) Init(ctx context.Context) error {
	// Initial semaphore is open and allows only one holder
	sem := Semaphore{
		Index:     0,
		Semaphore: 1,
		Max:       1,
		Holders:   nil,
	}
	b, err := json.Marshal(sem)
	if err != nil {
		return err
	}

	// If the key exists, then another client initialized already.
	if _, err := c.keyapi.Create(ctx, c.keypath, string(b)); err != nil {
		eerr, ok := err.(client.Error)
		if ok && eerr.Code == client.ErrorCodeNodeExist {
			return nil
		}

		return err
	}

	return nil
}

// Get fetches the Semaphore from etcd.
func (c *EtcdLockClient) Get(ctx context.Context) (*Semaphore, error) {
	resp, err := c.keyapi.Get(ctx, c.keypath, nil)
	if err != nil {
		return nil, err
	}

	sem := &Semaphore{}
	err = json.Unmarshal([]byte(resp.Node.Value), sem)
	if err != nil {
		return nil, err
	}

	sem.Index = resp.Node.ModifiedIndex

	return sem, nil
}

// Set sets a Semaphore in etcd.
func (c *EtcdLockClient) Set(ctx context.Context, sem *Semaphore) error {
	if sem == nil {
		return errors.New("cannot set nil semaphore")
	}
	b, err := json.Marshal(sem)
	if err != nil {
		return err
	}

	setopts := &client.SetOptions{
		PrevIndex: sem.Index,
	}

	_, err = c.keyapi.Set(ctx, c.keypath, string(b), setopts)
	return err
}
