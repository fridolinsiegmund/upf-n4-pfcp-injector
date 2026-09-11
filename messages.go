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
	"fmt"
	"net"
	"time"

	"github.com/wmnsk/go-pfcp/ie"
	"github.com/wmnsk/go-pfcp/message"
)

var recoveryTimeStamp = time.Now()

func create_assoc_request(own_ip *string) *message.AssociationSetupRequest {
	var sequenceNumber uint32 = 42
	assocSetupReq := message.NewAssociationSetupRequest(
		sequenceNumber,
		ie.NewNodeID(*own_ip, "", ""),
		ie.NewRecoveryTimeStamp(recoveryTimeStamp),
		ie.NewCPFunctionFeatures(0x0),
	)

	//log.Printf("assocSetupReq Flags= %x, string= %s", assocSetupReq.Header.Flags, assocSetupReq.Header.Flags)

	return assocSetupReq
}

// pfcp required the string to start with the length of the string as byte
func getNWInstance(instance string) string {
	// Prepend to string the length of the string as utf-8 encoded str (so its decoded later by framework)
	ch := rune(len(instance))
	return fmt.Sprintf("%c%s", ch, instance)
}

func create_PDI(cfg config) *ie.IE {
	var fteid *ie.IE = nil
	// if !cfg.remove_header {
	// 	fteid = ie.NewFTEID(0x0f, cfg.teid, net.ParseIP("127.0.0.1"), nil, 0x05)
	// }
	fteid = cfg.fteid

	var pdi *ie.IE
	if cfg.qfid != nil {
		pdi = ie.NewPDI(
			ie.NewSourceInterface(cfg.source_iface),
			//ie.NewFTEID(0x0f, teid, net.ParseIP("127.0.0.1"), nil, 0x05), //TODO: chid correct? teid in all cases?
			fteid, //ie.NewFTEID(0x0f, 0, net.ParseIP("127.0.0.1"), nil, 0x05),
			ie.NewNetworkInstance(getNWInstance("internet")),
			cfg.qfid,
			// ie.NewRedundantTransmissionParametersInPDI(
			// 	ie.NewFTEID(0x01, 0x11111111, net.ParseIP("127.0.0.1"), nil, 0),
			// 	ie.NewNetworkInstance("some.instance.example"),
			// ),
			// ie.NewUEIPAddress(0x06, UE_IP_ADDR, "", 0, 0), // flags 0x06 means V4 present and S/D Destination IP address present
			ie.NewTGPPInterfaceType(11), // 0=spare, 11 = N3
		)
	} else {
		if cfg.source_iface == ie.SrcInterfaceCPFunction {
			pdi = ie.NewPDI(
				ie.NewSourceInterface(cfg.source_iface),
				fteid,
			)
		} else {
			pdi = ie.NewPDI(
				ie.NewSourceInterface(cfg.source_iface),
				fteid,
				ie.NewNetworkInstance(getNWInstance("internet")),
				// ie.NewRedundantTransmissionParametersInPDI(
				// 	ie.NewFTEID(0x01, 0x11111111, net.ParseIP("127.0.0.1"), nil, 0),
				// 	ie.NewNetworkInstance("some.instance.example"),
				// ),
				ie.NewUEIPAddress(0x06, cfg.ue_ip, "", 0, 0),  // flags 0x06 means V4 present and S/D Destination IP address present
				ie.NewTGPPInterfaceType((0b0001<<6)+0b010001), // 1=spare, 17 = N6
			)
		}
	}

	return pdi
}

// ue_ip string, pdr_id uint16, source_iface uint8, teid uint32, qfid uint8, remove_header bool
func create_PDR_IE(cfg config) *ie.IE {
	var pdr *ie.IE

	if cfg.remove_header {
		var qer_id *ie.IE
		if cfg.qer_id != 0 {
			qer_id = ie.NewQERID(cfg.qer_id)
		} else {
			qer_id = nil
		}
		pdr = ie.NewCreatePDR(
			ie.NewPDRID(cfg.pdr_id),
			ie.NewPrecedence(cfg.precedence),
			create_PDI(cfg),
			ie.NewOuterHeaderRemoval(0x00, 0x01), // second val: 0x00 = no GTP PDU Session container removal, 0x01 = remove
			ie.NewFARID(cfg.far_id),
			// ie.NewURRID(cfg.urr_id),
			qer_id,
			// ie.NewQERID(cfg.qer_id),
		)
	} else {
		var urr_id *ie.IE
		if cfg.urr_id != 0 {
			urr_id = ie.NewURRID(cfg.urr_id)
		} else {
			urr_id = nil
		}
		pdr = ie.NewCreatePDR(
			ie.NewPDRID(cfg.pdr_id),
			ie.NewPrecedence(cfg.precedence),
			create_PDI(cfg),
			// ie.NewOuterHeaderRemoval(0x00, 0x00),
			ie.NewFARID(cfg.far_id),
			urr_id,
			ie.NewQERID(cfg.qer_id),
		)
	}

	return pdr
}

// UE_IP_ADDR string, GNB_IP_ADDR string, UPF_N3_IP string, TEID uint32, MBR_DL uint64, MBR_UL uint64
func create_session_establishment_request(sess session) *message.SessionEstablishmentRequest {
	UE_IP_ADDR := sess.ue_ip
	GNB_IP_ADDR := sess.gnb_ip
	UPF_N3_IP := sess.upf_n3_ip
	TEID := sess.teid
	MBR_DL := sess.mbr_dl
	MBR_UL := sess.mbr_ul

	// CONFIGURATION
	// Create two PDRs with different configurations
	var cfg1 config
	cfg1.ue_ip = UE_IP_ADDR
	cfg1.gnb_ip = GNB_IP_ADDR
	cfg1.pdr_id = 0x1
	cfg1.precedence = 65535
	cfg1.source_iface = ie.SrcInterfaceCore
	cfg1.teid = TEID
	cfg1.qfid = nil
	cfg1.remove_header = false
	cfg1.far_id = 0x1
	cfg1.urr_id = 0x0 //changed for DPDK UPF from 0x1
	cfg1.qer_id = 0x1
	cfg1.fteid = nil

	var cfg2 config
	cfg2.ue_ip = UE_IP_ADDR
	cfg2.gnb_ip = GNB_IP_ADDR
	cfg2.pdr_id = 0x2
	cfg2.precedence = 65535
	cfg2.source_iface = ie.SrcInterfaceAccess
	cfg2.teid = TEID
	cfg2.qfid = ie.NewQFI(0x1)
	cfg2.remove_header = true
	cfg2.far_id = 0x2
	cfg2.urr_id = 0x0 // 0 = not set
	cfg2.qer_id = 0x1
	cfg2.fteid = ie.NewFTEID(0x01, TEID, net.ParseIP(UPF_N3_IP), nil, 0x0) // 0x1 "ipv4 present", flags changed from 0x0f to 0x01, chose id was 0x05, UE or GNB IP?

	// currently not used, test for open5gs upf
	var cfg3 config
	cfg3.ue_ip = UE_IP_ADDR
	cfg3.gnb_ip = GNB_IP_ADDR
	cfg3.pdr_id = 0x3
	cfg3.precedence = 255
	cfg3.source_iface = ie.SrcInterfaceCPFunction
	cfg3.teid = TEID
	cfg3.qfid = nil
	cfg3.remove_header = true
	cfg3.far_id = 0x1
	cfg3.urr_id = 0x0 // 0 = not set
	cfg3.qer_id = 0x0 // 0 = not set
	cfg3.fteid = ie.NewFTEID(0x07, TEID, nil, nil, 0x0)

	// currently not used, test for open5gs upf
	var cfg4 config
	cfg4.ue_ip = UE_IP_ADDR
	cfg4.gnb_ip = GNB_IP_ADDR
	cfg4.pdr_id = 0x4
	cfg4.precedence = 255
	cfg4.source_iface = ie.SrcInterfaceAccess
	cfg4.teid = TEID
	cfg4.qfid = ie.NewSDFFilter("permit out 58 from ff02::2/128 to assigned", "", "", "", 0) //ie.NewQFI(0x2)
	cfg4.remove_header = true
	cfg4.far_id = 0x3
	cfg4.urr_id = 0x0 // 0 = not set
	cfg4.qer_id = 0x0 // 0 = not set
	cfg4.fteid = ie.NewFTEID(0x0f, TEID, net.ParseIP(sess.own_ip), nil, 0x05)

	var mbr_ie *ie.IE = nil
	if MBR_UL > 0 || MBR_DL > 0 {
		mbr_ie = ie.NewMBR(MBR_UL, MBR_DL)
	}

	var mp uint8 = 0x0 // Message Priority Option
	var fo uint8 = 0x0 // Follow On Option
	var seid uint64 = 0x0
	var sequenceNumber uint32 = sess.seq
	var spare uint8 = 0
	sessEstablishmentRequest := message.NewSessionEstablishmentRequest(
		mp,
		fo,
		seid,
		sequenceNumber,
		spare,
		ie.NewNodeID(sess.own_ip, "", ""),
		ie.NewFSEID(sess.seid, net.ParseIP(sess.own_ip), nil),
		create_PDR_IE(cfg1),
		create_PDR_IE(cfg2),
		// create_PDR_IE(cfg3),
		// create_PDR_IE(cfg4), TEST
		// original FAR from first open5gs session establishement request
		// ie.NewCreateFAR(
		// 	ie.NewFARID(cfg1.far_id),
		// 	ie.NewApplyAction(0xc, 0x0),
		// 	ie.NewBARID(0x1),
		// ),
		// Merge Session Mod Request from original PCAP
		// Also merged second Mod request from original PCAP
		ie.NewCreateFAR( // MERGE Session Mod Request from original PCAP
			ie.NewFARID(cfg1.far_id),
			ie.NewApplyAction(0x2, 0x0), //0x2 = Forwarding, 0xc = Notify CP and Buffering
			ie.NewBARID(0x1),
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceAccess),
				ie.NewNetworkInstance(getNWInstance("internet")),
				ie.NewOuterHeaderCreation(256, cfg1.teid, cfg1.gnb_ip, "", 0x00, 0x00, 0x00), //256=GTPU/UDP/IPV4
				ie.NewTGPPInterfaceType((0b0000<<6)+0b001011),                                // 0=spare, 11 = N3
			),
		),
		ie.NewCreateFAR(
			ie.NewFARID(cfg2.far_id),
			ie.NewApplyAction(0x2, 0x0),
			ie.NewForwardingParameters(
				ie.NewDestinationInterface(ie.DstInterfaceCore),
				ie.NewNetworkInstance(getNWInstance("internet")),
				ie.NewTGPPInterfaceType((0b0001<<6)+0b010001), // 1=spare, 17 = N6
			),
		),
		ie.NewCreateQER(
			ie.NewQERID(cfg1.qer_id),
			ie.NewGateStatus(0x0, 0x0),
			mbr_ie,         //ie.NewMBR(MBR_UL, MBR_DL),
			ie.NewQFI(0x1), //cfg2.qfid
		),
		ie.NewCreateBAR(
			ie.NewBARID(0x1),
		),
		ie.NewPDNType(ie.PDNTypeIPv4),
		// TODO: user ID changing IMEI/IMSI per session?
		// ie.NewUserID(
		// 	0x03,               // flags
		// 	"1234567891234567",  // IMSI
		// 	"9876543219876543", // IMEI
		// 	"1234567890123456", // MSISDN
		// 	"1234567890",       // NAI
		// ),
		ie.NewAPNDNN("internet"),
		ie.NewSNSSAI(0x01, 0xffffff),
	)

	//log.Printf("sessEstablishmentRequest Flags= %x, string= %s", sessEstablishmentRequest.Header.Flags, sessEstablishmentRequest.Header.Flags)

	return sessEstablishmentRequest

}
