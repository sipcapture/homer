// Copyright (C) 2025 Homer Server Contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.
//

package decoder

import (
	"encoding/binary"
	"fmt"
	"strconv"
)

func (h *HEP) parseHEP(packet []byte) error {
	length := binary.BigEndian.Uint16(packet[4:6])
	if int(length) != len(packet) {
		return fmt.Errorf("HEP packet length is %d but should be %d", len(packet), length)
	}
	currentByte := uint16(6)

	for currentByte < length {
		hepChunk := packet[currentByte:]
		if len(hepChunk) < 6 {
			return fmt.Errorf("HEP chunk must be >= 6 byte long but is %d", len(hepChunk))
		}
		//chunkVendorId := binary.BigEndian.Uint16(hepChunk[:2])
		chunkType := binary.BigEndian.Uint16(hepChunk[2:4])
		chunkLength := binary.BigEndian.Uint16(hepChunk[4:6])
		if len(hepChunk) < int(chunkLength) || int(chunkLength) < 6 {
			return fmt.Errorf("HEP chunk with %d byte < chunkLength %d or chunkLength < 6", len(hepChunk), chunkLength)
		}
		chunkBody := hepChunk[6:chunkLength]

		switch chunkType {
		case Version, Protocol, ProtoType:
			if len(chunkBody) != 1 {
				return fmt.Errorf("HEP chunkType %d should be 1 byte long but is %d", chunkType, len(chunkBody))
			}
		case SrcPort, DstPort, Vlan:
			if len(chunkBody) != 2 {
				return fmt.Errorf("HEP chunkType %d should be 2 byte long but is %d", chunkType, len(chunkBody))
			}
		case IP4SrcIP, IP4DstIP, Tsec, Tmsec, NodeID:
			if len(chunkBody) != 4 {
				return fmt.Errorf("HEP chunkType %d should be 4 byte long but is %d", chunkType, len(chunkBody))
			}
		case IP6SrcIP, IP6DstIP:
			if len(chunkBody) != 16 {
				return fmt.Errorf("HEP chunkType %d should be 16 byte long but is %d", chunkType, len(chunkBody))
			}
		}

		switch chunkType {
		case Version:
			h.Version = uint32(chunkBody[0])
		case Protocol:
			h.Protocol = uint32(chunkBody[0])
		case IP4SrcIP:
			h.SrcIP = ipv4BytesToString(chunkBody)
		case IP4DstIP:
			h.DstIP = ipv4BytesToString(chunkBody)
		case IP6SrcIP:
			h.SrcIP = ipv6BytesToString(chunkBody)
		case IP6DstIP:
			h.DstIP = ipv6BytesToString(chunkBody)
		case SrcPort:
			h.SrcPort = uint32(binary.BigEndian.Uint16(chunkBody))
		case DstPort:
			h.DstPort = uint32(binary.BigEndian.Uint16(chunkBody))
		case Tsec:
			h.Tsec = binary.BigEndian.Uint32(chunkBody)
		case Tmsec:
			h.Tmsec = binary.BigEndian.Uint32(chunkBody)
		case ProtoType:
			h.ProtoType = uint32(chunkBody[0])
			switch h.ProtoType {
			case 1:
				h.ProtoString = "sip"
			case 5:
				h.ProtoString = "rtcp"
			case 34:
				h.ProtoString = "rtpagent"
			case 35:
				h.ProtoString = "rtcpxr"
			case 38:
				h.ProtoString = "horaclifix"
			case 53:
				h.ProtoString = "dns"
			case 56:
				h.ProtoString = "diameter"
			case 100:
				h.ProtoString = "log"
			default:
				h.ProtoString = strconv.Itoa(int(h.ProtoType))
			}
		case NodeID:
			h.NodeID = binary.BigEndian.Uint32(chunkBody)
		case NodePW:
			h.NodePW = string(chunkBody)
		case Payload:
			h.Payload = string(chunkBody)
		case CID:
			h.CID = string(chunkBody)
		case Vlan:
			h.Vlan = uint32(binary.BigEndian.Uint16(chunkBody))
		case NodeName:
			h.NodeName = string(chunkBody)
		default:
		}
		currentByte += chunkLength
	}
	return nil
}

func (h *HEP) parseHEP2(packet []byte) error {

	h.ProtoString = "sip"
	h.ProtoType = 1

	h.Version = uint32(packet[0])
	//length := packet[1]
	family := packet[2]
	h.Protocol = uint32(packet[3])
	h.SrcPort = uint32(binary.BigEndian.Uint16(packet[4:6]))
	h.DstPort = uint32(binary.BigEndian.Uint16(packet[6:8]))
	totalLen := 8

	if family == 10 {
		h.SrcIP = ipv6BytesToString(packet[8:24])
		h.DstIP = ipv6BytesToString(packet[24:40])
		totalLen += 32
	} else {
		h.SrcIP = ipv4BytesToString(packet[8:12])
		h.DstIP = ipv4BytesToString(packet[12:16])
		totalLen += 8
	}

	if h.Version == 2 {
		h.Tsec = binary.BigEndian.Uint32(packet[totalLen:(totalLen + 4)])
		totalLen += 4
		h.Tmsec = binary.BigEndian.Uint32(packet[totalLen:(totalLen + 4)])
		totalLen += 4
		h.CID = string(packet[totalLen:(totalLen + 2)])
		totalLen += 2
	}

	h.Payload = string(packet[totalLen:])

	return nil
}
