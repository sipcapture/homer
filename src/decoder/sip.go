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
	"errors"
	"unsafe"

	"github.com/sipcapture/homer-core/src/homerconfig"
	"github.com/sipcapture/homer-core/src/sipparser"
)

// stob converts string to []byte without copying (zero-copy).
// The returned slice must not be modified.
func stob(s string) []byte {
	return unsafe.Slice(unsafe.StringData(s), len(s))
}

// parseSIP decodes the SIP payload. When ingest lists aleg_ids / custom_headers,
// they are passed into ParseMsgZeroCopy so XCallID and CustomHeader are filled
// before CID selection (see sipparser.ZeroCopyOpts).
func (h *HEP) parseSIP() error {
	var forceALegID bool
	var zcOpts *sipparser.ZeroCopyOpts

	if h.decoder != nil && h.decoder.config != nil {
		cfg := h.decoder.config
		forceALegID = cfg.ForceALegID
		if len(cfg.AlegIDs) > 0 || len(cfg.CustomHeaders) > 0 {
			zcOpts = &sipparser.ZeroCopyOpts{AlegIDs: cfg.AlegIDs, CustomHeaders: cfg.CustomHeaders}
		}
	} else if homerconfig.MainConfig != nil {
		sip := homerconfig.MainConfig.Setting.SIP_SETTINGS
		forceALegID = sip.ForceALegID
		if len(sip.AlegIDs) > 0 || len(sip.CustomHeaders) > 0 {
			zcOpts = &sipparser.ZeroCopyOpts{AlegIDs: sip.AlegIDs, CustomHeaders: sip.CustomHeaders}
		}
	}

	h.SIP = sipparser.ParseMsgZeroCopy(stob(h.Payload), zcOpts)

	if h.SIP.Error != nil {
		return h.SIP.Error
	} else if len(h.SIP.CseqMethod) < 3 {
		return errors.New("could not find a valid CSeq in packet")
	} else if len(h.SIP.CallID) < 1 {
		return errors.New("could not find a valid Call-ID in packet")
	}
	if h.SIP.FirstMethod == "" {
		h.SIP.FirstMethod = h.SIP.FirstResp
	}

	switch h.SIP.CseqMethod {
	case "INVITE", "ACK", "BYE", "CANCEL", "UPDATE", "PRACK", "REFER", "INFO":
		h.SIP.Profile = "call"
	case "REGISTER":
		h.SIP.Profile = "registration"
	default:
		h.SIP.Profile = "default"
	}

	if h.CID == "" {
		if h.SIP.XCallID != "" {
			h.CID = h.SIP.XCallID
		} else {
			h.CID = h.SIP.CallID
		}
	} else if forceALegID && h.SIP.XCallID != "" {
		h.CID = h.SIP.XCallID
	}

	h.SID = h.SIP.CallID

	return nil
}
