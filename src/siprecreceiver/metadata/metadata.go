// Package metadata implements parsing of SIPREC recording metadata (RFC 7865).
package metadata

import (
	"encoding/xml"
	"fmt"
	"strings"
)

const Namespace = "urn:ietf:params:xml:ns:recording:1"
const ContentType = "application/rs-metadata+xml"

type Recording struct {
	XMLName  xml.Name `xml:"recording" json:"-"`
	DataMode string   `xml:"datamode,omitempty" json:"datamode,omitempty"`

	Groups       []Group       `xml:"group" json:"groups,omitempty"`
	Sessions     []Session     `xml:"session" json:"sessions,omitempty"`
	Participants []Participant `xml:"participant" json:"participants,omitempty"`
	Streams      []Stream      `xml:"stream" json:"streams,omitempty"`

	SessionRecordingAssocs   []SessionRecordingAssoc   `xml:"sessionrecordingassoc" json:"session_recording_assocs,omitempty"`
	ParticipantSessionAssocs []ParticipantSessionAssoc `xml:"participantsessionassoc" json:"participant_session_assocs,omitempty"`
	ParticipantStreamAssocs  []ParticipantStreamAssoc  `xml:"participantstreamassoc" json:"participant_stream_assocs,omitempty"`
}

type Group struct {
	ID            string `xml:"group_id,attr" json:"group_id"`
	AssociateTime string `xml:"associate-time,omitempty" json:"associate_time,omitempty"`
}

type Session struct {
	ID            string   `xml:"session_id,attr" json:"session_id"`
	SIPSessionIDs []string `xml:"sipSessionID" json:"sip_session_ids,omitempty"`
	GroupRef      string   `xml:"group-ref,omitempty" json:"group_ref,omitempty"`
	Reason        string   `xml:"reason,omitempty" json:"reason,omitempty"`
	StartTime     string   `xml:"start-time,omitempty" json:"start_time,omitempty"`
	StopTime      string   `xml:"stop-time,omitempty" json:"stop_time,omitempty"`
}

type Participant struct {
	ID      string   `xml:"participant_id,attr" json:"participant_id"`
	NameIDs []NameID `xml:"nameID" json:"name_ids,omitempty"`
}

type NameID struct {
	AoR  string `xml:"aor,attr" json:"aor"`
	Name string `xml:"name,omitempty" json:"name,omitempty"`
}

func (p Participant) DisplayName() string {
	for _, n := range p.NameIDs {
		if n.Name != "" {
			return n.Name
		}
	}
	for _, n := range p.NameIDs {
		if n.AoR != "" {
			return n.AoR
		}
	}
	return p.ID
}

type Stream struct {
	ID        string `xml:"stream_id,attr" json:"stream_id"`
	SessionID string `xml:"session_id,attr" json:"session_id"`
	Label     string `xml:"label,omitempty" json:"label,omitempty"`
}

type SessionRecordingAssoc struct {
	SessionID     string `xml:"session_id,attr" json:"session_id"`
	AssociateTime string `xml:"associate-time,omitempty" json:"associate_time,omitempty"`
	FixedTime     string `xml:"disassociate-time,omitempty" json:"disassociate_time,omitempty"`
}

type ParticipantSessionAssoc struct {
	ParticipantID    string `xml:"participant_id,attr" json:"participant_id"`
	SessionID        string `xml:"session_id,attr" json:"session_id"`
	AssociateTime    string `xml:"associate-time,omitempty" json:"associate_time,omitempty"`
	DisassociateTime string `xml:"disassociate-time,omitempty" json:"disassociate_time,omitempty"`
}

type ParticipantStreamAssoc struct {
	ParticipantID string   `xml:"participant_id,attr" json:"participant_id"`
	Send          []string `xml:"send" json:"send,omitempty"`
	Recv          []string `xml:"recv" json:"recv,omitempty"`
}

func Parse(data []byte) (*Recording, error) {
	var rec Recording
	if err := xml.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("metadata: invalid XML: %w", err)
	}
	if rec.XMLName.Space != "" && rec.XMLName.Space != Namespace {
		return nil, fmt.Errorf("metadata: unexpected XML namespace %q", rec.XMLName.Space)
	}
	return &rec, nil
}

func (r *Recording) PrimarySIPSessionID() string {
	for _, s := range r.Sessions {
		for _, id := range s.SIPSessionIDs {
			if id != "" {
				return id
			}
		}
	}
	return ""
}

func (r *Recording) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "datamode=%s sessions=%d participants=%d streams=%d",
		r.DataMode, len(r.Sessions), len(r.Participants), len(r.Streams))
	return b.String()
}
