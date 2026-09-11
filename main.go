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
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	var o options
	flag.StringVar(&o.upfN4, "upf_n4_ip", "127.0.0.8:8805", "upf n4 addr/port")
	flag.StringVar(&o.gnbIP, "gnb_ip", "10.42.10.11", "gnb ip")
	flag.StringVar(&o.upfN3, "upf_n3_ip", "10.42.10.1", "upf n3 ip")
	flag.StringVar(&o.ownIP, "own_ip", "127.0.0.1", "own ip")
	flag.IntVar(&o.numSessions, "num_sess", 10000, "number of sessions, starting at UE IP 10.60.0.1 and TEID=1")
	flag.Int64Var(&o.mbrDL, "mbr_dl", 1000000000, "downlink MBR in Kbit/s per session")
	flag.Int64Var(&o.mbrUL, "mbr_ul", 1000000000, "uplink MBR in Kbit/s per session")
	flag.BoolVar(&o.alternateMBR, "ab_mbr", false, "multiply MBR by 10 for every second session")
	flag.DurationVar(&o.timeout, "request_timeout", 3*time.Second, "response timeout per attempt")
	flag.IntVar(&o.retries, "retries", 2, "retransmissions after the initial attempt")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, o); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
