// Copyright (C) 2026 Homer Server Contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package siprecreceiver

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"

	"github.com/sipcapture/homer-core/src/siprecreceiver/metadata"
	"github.com/sipcapture/homer-core/src/siprecreceiver/sdpx"
)

type inviteBody struct {
	SDP         []byte
	MetadataXML []byte
}

func parseInviteBody(contentType string, body []byte) (*inviteBody, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("empty request body")
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Type %q: %w", contentType, err)
	}
	switch {
	case mediaType == sdpx.ContentType:
		return &inviteBody{SDP: body}, nil
	case strings.HasPrefix(mediaType, "multipart/"):
		boundary := params["boundary"]
		if boundary == "" {
			return nil, fmt.Errorf("multipart body without boundary")
		}
		return parseMultipart(body, boundary)
	default:
		return nil, fmt.Errorf("unsupported Content-Type %q", mediaType)
	}
}

func parseMultipart(body []byte, boundary string) (*inviteBody, error) {
	out := &inviteBody{}
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("malformed multipart body: %w", err)
		}
		data, err := io.ReadAll(part)
		part.Close()
		if err != nil {
			return nil, fmt.Errorf("read multipart part: %w", err)
		}
		partType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch partType {
		case sdpx.ContentType:
			out.SDP = data
		case metadata.ContentType, "application/rs-metadata":
			out.MetadataXML = data
		}
	}
	if out.SDP == nil {
		return nil, fmt.Errorf("multipart body contains no application/sdp part")
	}
	return out, nil
}
