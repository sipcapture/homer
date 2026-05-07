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

package errorcodes

// Messages Codes for Errors
const (
	UserRequestFailed             = 1002
	UserSettingsFailed            = 1003
	MappingHepSubFailed           = 1004
	InsertDashboardFailed         = 1005
	GetDashboardFailed            = 1006
	GetDashboardListFailed        = 1007
	GetDBNodeListFailed           = 1008
	GetAuthTypeListFailed         = 1009
	DeleteDashboardFailed         = 1010
	MappingSchemaFailed           = 1011
	HepSubRequestFailed           = 1012
	GetAgentSubFailed             = 1013
	GetAuthTokenFailed            = 1014
	DeleteAdvancedAgainstFailed   = 1015
	GetAdvancedAgainstFailed      = 1016
	MappingSchemaByUUIDFailed     = 1017
	MappingRecreateFailed         = 1018
	DeleteMappingSchemaFailed     = 1019
	SmartHepProfileFailed         = 1020
	DashboardNotExists            = 1021
	HomeDashboardNotExists        = 1022
	Unauthorized                  = 1023
	IncorrectPassword             = 1024
	UserCreationFailed            = 1025
	UserDeleteionFailed           = 1026
	SuccessfullyCreatedUser       = 1027
	UserRequestFormatIncorrect    = 1028
	UserGroupInformationError     = 1029
	ResyncFormatIncorrect         = 1030
	ResyncBadSourceNode           = 1031
	FileRequestFormatNotFound     = 1032
	AgentStorageNotFound          = 1033
	StenographerException         = 1034
	UserRequestBadValueInside     = 1035
	ShareLinkGeneratorError       = 1036
	ShareLinkNotExistsError       = 1037
	ShareLinkBadDataError         = 1038
	UserReportZipGenerationError  = 1039
	UIVersionFileNotExistsError   = 1040
	MarshalingConvertionsError    = 1041
	BadPcapDecodingError          = 1042
	ExportConfigNotActiveError    = 1043
	BackupFileCreateError         = 1044
	BadValidationForParamterers   = 1045
	DownloadLogFileNotActiveError = 1046
	StenographerProxyException    = 1047
	GrafanaProcessingError        = 1048
	CallIDNotExist                = 1049
	UknownError                   = 5001
)
