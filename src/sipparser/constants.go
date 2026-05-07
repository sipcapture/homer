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

package sipparser

// constants just holds a common shared set of constants (i.e. hdr values)
const (
	// SIP request or response
	SIP_REQUEST  = "REQUEST"
	SIP_RESPONSE = "RESPONSE"
	// SIP Methods
	SIP_METHOD_INVITE    = "INVITE"
	SIP_METHOD_ACK       = "ACK"
	SIP_METHOD_OPTIONS   = "OPTIONS"
	SIP_METHOD_BYE       = "BYE"
	SIP_METHOD_CANCEL    = "CANCEL"
	SIP_METHOD_REGISTER  = "REGISTER"
	SIP_METHOD_INFO      = "INFO"
	SIP_METHOD_PRACK     = "PRACK"
	SIP_METHOD_SUBSCRIBE = "SUBSCRIBE"
	SIP_METHOD_NOTIFY    = "NOTIFY"
	SIP_METHOD_UPDATE    = "UPDATE"
	SIP_METHOD_MESSAGE   = "MESSAGE"
	SIP_METHOD_REFER     = "REFER"
	SIP_METHOD_PUBLISH   = "PUBLISH"
	// SIP Headers
	SIP_HDR_ACCEPT                        = "accept"         // RFC3261
	SIP_HDR_ACCEPT_CONTACT                = "accept-contact" // RFC3841
	SIP_HDR_ACCEPT_CONTACT_CMP            = "a"              // RFC3841
	SIP_HDR_ACCEPT_ENCODING               = "accept-encoding"
	SIP_HDR_ACCEPT_LANGUAGE               = "accept-language"
	SIP_HDR_ACCEPT_RESOURCE_PRIORITY      = "accept-resource-priority" // RFC4412
	SIP_HDR_ALERT_INFO                    = "alert-info"
	SIP_HDR_ALLOW                         = "allow"
	SIP_HDR_ALLOW_EVENTS                  = "allow-events"
	SIP_HDR_ALLOW_EVENTS_CMP              = "u"
	SIP_HDR_ANSWER_MODE                   = "answer-mode"
	SIP_HDR_AUTHENTICATION_INFO           = "authentication-info"
	SIP_HDR_AUTHORIZATION                 = "authorization"
	SIP_HDR_CALL_ID                       = "call-id"
	SIP_HDR_CALL_ID_CMP                   = "i"
	SIP_HDR_CALL_INFO                     = "call-info"
	SIP_HDR_CONTACT                       = "contact"
	SIP_HDR_CONTACT_CMP                   = "m"
	SIP_HDR_CONTENT_DISPOSITION           = "content-disposition"
	SIP_HDR_CONTENT_ENCODING              = "content-encoding"
	SIP_HDR_CONTENT_ENCODING_CMP          = "e"
	SIP_HDR_CONTENT_LANGUAGE              = "content-language"
	SIP_HDR_CONTENT_LENGTH                = "content-length"
	SIP_HDR_CONTENT_LENGTH_CMP            = "l"
	SIP_HDR_CONTENT_TYPE                  = "content-type"
	SIP_HDR_CONTENT_TYPE_CMP              = "c"
	SIP_HDR_CSEQ                          = "cseq"
	SIP_HDR_DATE                          = "date"
	SIP_HDR_ERROR_INFO                    = "error-info"
	SIP_HDR_EVENT                         = "event"
	SIP_HDR_EXPIRES                       = "expires"
	SIP_HDR_FLOW_TIMER                    = "flow-timer"
	SIP_HDR_FROM                          = "from"
	SIP_HDR_FROM_CMP                      = "f"
	SIP_HDR_HISTORY_INFO                  = "history-info"  // RFC 4244
	SIP_HDR_IDENTITY                      = "identity"      // RFC 4474
	SIP_HDR_IDENTITY_CMP                  = "y"             // RFC 4474
	SIP_HDR_IDENTITY_INFO                 = "identity-info" // RFC 4474
	SIP_HDR_IDENTITY_INFO_CMP             = "n"             // RFC 4474
	SIP_HDR_IN_REPLY_TO                   = "in-reply-to"
	SIP_HDR_JOIN                          = "join" // RFC 3911
	SIP_HDR_MAX_FORWARDS                  = "max-forwards"
	SIP_HDR_MIME_VERSION                  = "mime-version"
	SIP_HDR_MIN_EXPIRES                   = "min-expires"
	SIP_HDR_MIN_SE                        = "min-se" // RFC4028
	SIP_HDR_ORGANIZATION                  = "organization"
	SIP_HDR_PATH                          = "path"               // RFC3327
	SIP_HDR_PERMISSION_MISSING            = "permission-missing" // RFC5360
	SIP_HDR_PRIORITY                      = "priority"
	SIP_HDR_PRIVACY                       = "privacy"
	SIP_HDR_PRIV_ANSWER_MODE              = "priv-answer-mode" // RFC 5373
	SIP_HDR_PROXY_AUTHENTICATE            = "proxy-authenticate"
	SIP_HDR_PROXY_AUTHORIZATION           = "proxy-authorization"
	SIP_HDR_PROXY_REQUIRE                 = "proxy-require"
	SIP_HDR_RACK                          = "rack" // RFC 3262
	SIP_HDR_REASON                        = "reason"
	SIP_HDR_RECORD_ROUTE                  = "record-route"
	SIP_HDR_REFER_SUB                     = "refer-sub"                     // RFC4488
	SIP_HDR_REFER_TO                      = "refer-to"                      // RFC 3515, RFC 4508
	SIP_HDR_REFERRED_BY                   = "referred-by"                   // RFC3892
	SIP_HDR_REFERRED_BY_CMP               = "b"                             // RFC3892
	SIP_HDR_REJECT_CONTACT                = "reject-contact"                // RFC3841
	SIP_HDR_REJECT_CONTACT_CMP            = "j"                             // RFC3841
	SIP_HDR_REMOTE_PARTY_ID               = "remote-party-id"               // DRAFT
	SIP_HDR_DIVERSION                     = "diversion"                     // RFC3261
	SIP_HDR_REPLACES                      = "replaces"                      // RFC3891
	SIP_HDR_REPLY_TO                      = "reply-to"                      // RFC3261
	SIP_HDR_REQUEST_DISPOSITION           = "request-disposition"           // RFC3841
	SIP_HDR_REQUIRE                       = "require"                       // RFC3261
	SIP_HDR_RESOURCE_PRIORITY             = "resource-priority"             // RFC4412
	SIP_HDR_RETRY_AFTER                   = "retry-after"                   // RFC3261
	SIP_HDR_ROUTE                         = "route"                         // RFC3261
	SIP_HDR_RSEQ                          = "rseq"                          // RFC3262
	SIP_HDR_SECUTIRY_CLIENT               = "security-client"               // RFC3329
	SIP_HDR_SECURITY_SERVER               = "security-server"               // RFC3329
	SIP_HDR_SECURITY_VERIFY               = "security-verify"               // RFC3329
	SIP_HDR_SERVER                        = "server"                        // RFC3261
	SIP_HDR_SERVICE_ROUTE                 = "service-route"                 // RFC3608
	SIP_HDR_SESSION_EXPIRES               = "session-expires"               // RFC4028
	SIP_HDR_SESSION_EXPIRES_CMP           = "x"                             // RFC4028
	SIP_HDR_SIP_ETAG                      = "sip-etag"                      // RFC3903
	SIP_HDR_SIP_IF_MATCH                  = "sip-if-match"                  // RFC3903
	SIP_HDR_SUBJECT                       = "subject"                       // RFC3261
	SIP_HDR_SUBJECT_CMP                   = "s"                             // RFC3261
	SIP_HDR_SUBSCRIPTION_STATE            = "subscription-state"            // RFC3265
	SIP_HDR_SUPPORTED                     = "supported"                     // RFC3261
	SIP_HDR_SUPPORTED_CMP                 = "k"                             // RFC3261
	SIP_HDR_SUPPRESS_IF_MATCH             = "suppress-if-match"             // RFC5839
	SIP_HDR_TARGET_DIALOG                 = "target-dialog"                 // RFC4538
	SIP_HDR_TIMESTAMP                     = "timestamp"                     // RFC3261
	SIP_HDR_TO                            = "to"                            // RFC3261
	SIP_HDR_TO_CMP                        = "t"                             // RFC3261
	SIP_HDR_TRIGGER_CONSENT               = "trigger-consent"               // RFC5360
	SIP_HDR_UNSUPPORTED                   = "unsupported"                   // RFC3261
	SIP_HDR_USER_AGENT                    = "user-agent"                    // RFC3261
	SIP_HDR_VIA                           = "via"                           // RFC3261
	SIP_HDR_VIA_CMP                       = "v"                             // RFC3261
	SIP_HDR_WARNING                       = "warning"                       // RFC3261
	SIP_HDR_WWW_AUTHENTICATE              = "www-authenticate"              // RFC3261
	SIP_HDR_P_ACCESS_NETWORK_INFO         = "p-access-network-info"         // RFC3455
	SIP_HDR_P_ANSWER_STATE                = "p-answer-state"                // RFC3455
	SIP_HDR_P_ASSERTED_IDENTITY           = "p-asserted-identity"           // RFC3325
	SIP_HDR_P_ASSERTED_SERVICE            = "p-asserted-service"            // RFC3455
	SIP_HDR_P_ASSOCIATED_URI              = "p-associated-uri"              // RFC3455
	SIP_HDR_P_CALLED_PARTY_ID             = "p-called-party-id"             // RFC3455
	SIP_HDR_P_CHARGING_FUNCTION_ADDRESSES = "p-charging-function-addresses" // RFC3455
	SIP_HDR_P_CHARGING_VECTOR             = "p-charging-vector"             // RFC3455
	SIP_HDR_P_DCS_BILLING_INFO            = "p-dcs-billing-info"            // RFC5503
	SIP_HDR_P_DCS_LAES                    = "p-dcs-laes"                    // RFC5503
	SIP_HDR_P_DCS_OSPS                    = "p-dcs-osps"                    // RFC5503
	SIP_HDR_P_DCS_REDIRECT                = "p-dcs-redirect"                // RFC5503
	SIP_HDR_P_DCS_TRACE_PARTY_ID          = "p-dcs-trace-party-id"          // RFC5503
	SIP_HDR_P_EARLY_MEDIA                 = "p-early-media"                 // RFC5009
	SIP_HDR_P_MEDIA_AUTHORIZATION         = "p-media-authorization"         // RFC3313
	SIP_HDR_P_PREFERRED_IDENTITY          = "p-preferred-identity"          // RFC3325
	SIP_HDR_P_PREFERRED_SERVICE           = "p-preferred-service"           // RFC6050
	SIP_HDR_P_PROFILE_KEY                 = "p-profile-key"                 // RFC5002
	// P-RTP-Stat is Cisco proprietary, see http://www.cisco.com/en/US/docs/ios-xml/ios/voice/cube_sip/configuration/15-2mt/voi-report-end-cal.html
	SIP_HDR_P_RTP_STAT           = "p-rtp-stat"
	SIP_HDR_P_USER_DATABASE      = "p-user-database"      // RFC4457
	SIP_HDR_P_VISITED_NETWORK_ID = "p-visited-network-id" // RFC3455
	// X-RTP-Stat is AVM proprietary, see https://avm.de/fileadmin/user_upload/Global/Service/Schnittstellen/xrtpv32.pdf
	SIP_HDR_X_RTP_STAT     = "x-rtp-stat"
	SIP_HDR_X_RTP_STAT_ADD = "x-rtp-stat-add"
)
