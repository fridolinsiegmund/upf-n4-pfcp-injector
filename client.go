// Copyright 2026-present Fridolin Siegmund
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

const (
	ipStart           = 0x0a3c0001
	maxSessions       = 0x0a3cfeff - ipStart
	maxMBR      int64 = 1<<40 - 1
)

type options struct {
	upfN4, gnbIP, upfN3, ownIP string
	numSessions                int
	mbrDL, mbrUL               int64
	alternateMBR               bool
	timeout                    time.Duration
	retries                    int
}

func (o options) validate() error {
	for name, value := range map[string]string{"own_ip": o.ownIP, "gnb_ip": o.gnbIP, "upf_n3_ip": o.upfN3} {
		if ip := net.ParseIP(value); ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("%s must be a unicast IPv4 address", name)
		}
	}
	if o.numSessions < 0 || o.numSessions > maxSessions {
		return fmt.Errorf("num_sess must be between 0 and %d", maxSessions)
	}
	limit := maxMBR
	if o.alternateMBR {
		limit /= 10
	}
	if o.mbrDL < 0 || o.mbrUL < 0 || o.mbrDL > limit || o.mbrUL > limit {
		return fmt.Errorf("MBR must be between 0 and %d Kbit/s with these settings", limit)
	}
	if o.timeout <= 0 || o.retries < 0 {
		return errors.New("request_timeout must be positive and retries must be nonnegative")
	}
	return nil
}

type responseKey struct {
	sequence uint32
	kind     uint8
}

type client struct {
	conn     *net.UDPConn
	mu       sync.Mutex
	pending  map[responseKey]chan message.Message
	done     chan struct{}
	readErr  error
	sessions map[uint64]sessionState
}

func newClient(conn *net.UDPConn) *client {
	return &client{conn: conn, pending: make(map[responseKey]chan message.Message), done: make(chan struct{})}
}

func (c *client) write(msg message.Message) error {
	b := make([]byte, msg.MarshalLen())
	if err := msg.MarshalTo(b); err != nil {
		return err
	}
	_, err := c.conn.Write(b)
	return err
}

func (c *client) readLoop() {
	defer close(c.done)
	buf := make([]byte, 65535)
	for {
		n, err := c.conn.Read(buf)
		if err != nil {
			c.readErr = err
			return
		}
		msg, err := message.Parse(append([]byte(nil), buf[:n]...))
		if err != nil {
			log.Printf("ignored undecodable message (%d bytes): %v", n, err)
			continue
		}
		switch req := msg.(type) {
		case *message.HeartbeatRequest:
			err = c.write(message.NewHeartbeatResponse(req.Sequence(), ie.NewRecoveryTimeStamp(recoveryTimeStamp)))
		case *message.SessionReportRequest:
			err = c.write(message.NewSessionReportResponse(0, 0, 0, req.Sequence(), 0))
		default:
			key := responseKey{msg.Sequence(), msg.MessageType()}
			c.mu.Lock()
			if ch := c.pending[key]; ch != nil {
				select {
				case ch <- msg:
				default:
				}
			}
			c.mu.Unlock()
		}
		if err != nil {
			c.readErr = err
			return
		}
	}
}

func (c *client) request(ctx context.Context, req message.Message, responseType uint8, timeout time.Duration, retries int) (message.Message, error) {
	key := responseKey{req.Sequence(), responseType}
	ch := make(chan message.Message, 1)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	b := make([]byte, req.MarshalLen())
	if err := req.MarshalTo(b); err != nil {
		return nil, err
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if _, err := c.conn.Write(b); err != nil {
			return nil, err
		}
		timer := time.NewTimer(timeout)
		select {
		case resp := <-ch:
			timer.Stop()
			return resp, nil
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-c.done:
			timer.Stop()
			return nil, c.readErr
		case <-timer.C:
		}
		if attempt == retries {
			return nil, fmt.Errorf("%s sequence %d: %w", req.MessageTypeName(), req.Sequence(), context.DeadlineExceeded)
		}
	}
}

func accepted(cause *ie.IE) error {
	if cause == nil {
		return errors.New("response is missing Cause")
	}
	value, err := cause.Cause()
	if err != nil {
		return fmt.Errorf("invalid Cause: %w", err)
	}
	if value != ie.CauseRequestAccepted {
		return fmt.Errorf("request rejected: Cause=%d", value)
	}
	return nil
}

type sessionState struct {
	session  session
	response *message.SessionEstablishmentResponse
}

func installSessions(ctx context.Context, c *client, o options) error {
	assoc := create_assoc_request(&o.ownIP)
	resp, err := c.request(ctx, assoc, message.MsgTypeAssociationSetupResponse, o.timeout, o.retries)
	if err != nil {
		return fmt.Errorf("association: %w", err)
	}
	if err := accepted(resp.(*message.AssociationSetupResponse).Cause); err != nil {
		return fmt.Errorf("association: %w", err)
	}

	seq := assoc.Sequence() + 1
	sessions := make(map[uint64]sessionState, o.numSessions)
	acceptedCount, rejected, failed := 0, 0, 0
	defer func() {
		log.Printf("Session batch finished: accepted=%d rejected=%d failed=%d requested=%d", acceptedCount, rejected, failed, o.numSessions)
	}()
	for index := 0; index < o.numSessions; index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var ip [4]byte
		binary.BigEndian.PutUint32(ip[:], uint32(ipStart+index))
		sess := session{ue_ip: net.IP(ip[:]).String(), gnb_ip: o.gnbIP, upf_n3_ip: o.upfN3, own_ip: o.ownIP,
			teid: uint32(index + 1), seid: uint64(index + 1), seq: seq, mbr_dl: uint64(o.mbrDL), mbr_ul: uint64(o.mbrUL)}
		seq++ // validated batch size must < 24 bit sequence
		if o.alternateMBR && index%2 == 1 {
			sess.mbr_dl *= 10
			sess.mbr_ul *= 10
		}
		reply, err := c.request(ctx, create_session_establishment_request(sess), message.MsgTypeSessionEstablishmentResponse, o.timeout, o.retries)
		if err != nil {
			failed++
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			log.Printf("UE %s: %v", sess.ue_ip, err)
			continue
		}
		established := reply.(*message.SessionEstablishmentResponse)
		if err := accepted(established.Cause); err != nil {
			rejected++
			log.Printf("UE %s: %v", sess.ue_ip, err)
			continue
		}
		sessions[sess.seid] = sessionState{sess, established}
		acceptedCount++
		log.Printf("Session accepted: UE=%s TEID=%d CP-SEID=%d", sess.ue_ip, sess.teid, sess.seid)
	}
	c.sessions = sessions
	return nil
}

func run(ctx context.Context, o options) error {
	if err := o.validate(); err != nil {
		return err
	}
	if o.numSessions == 0 {
		log.Print("No sessions requested")
		return nil
	}
	raddr, err := net.ResolveUDPAddr("udp4", o.upfN4)
	if err != nil {
		return err
	}
	if raddr.Port == 0 || raddr.IP == nil || raddr.IP.IsUnspecified() || raddr.IP.IsMulticast() {
		return errors.New("upf_n4_ip must specify a unicast IPv4 destination and nonzero port")
	}
	conn, err := net.DialUDP("udp4", &net.UDPAddr{IP: net.ParseIP(o.ownIP), Port: 8805}, raddr)
	if err != nil {
		return err
	}
	c := newClient(conn)
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	go c.readLoop()
	defer func() { conn.Close(); <-c.done }()
	if err := installSessions(ctx, c, o); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.readErr
	}
}
