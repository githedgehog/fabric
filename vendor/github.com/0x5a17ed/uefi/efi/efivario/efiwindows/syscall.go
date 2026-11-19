// Copyright (c) 2022 Arthur Skowronek <0x5a17ed@tuta.io> and contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// <https://www.apache.org/licenses/LICENSE-2.0>
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build windows

package efiwindows

import (
	"github.com/0x5a17ed/uefi/efi/efiguid"
)

//sys	GetFirmwareEnvironmentVariableEx(lpName *uint16, lpGuid *uint16, buf []byte, attrs *uint32) (n uint32, err error) = kernel32.GetFirmwareEnvironmentVariableExW
//sys	SetFirmwareEnvironmentVariableEx(lpName *uint16, lpGuid *uint16, buf []byte, attrs uint32) (err error) = kernel32.SetFirmwareEnvironmentVariableExW
//sys	NtEnumerateSystemEnvironmentValuesEx(InformationClass uint32, buf *byte, buflen *uint32) (ntstatus error) = ntdll.NtEnumerateSystemEnvironmentValuesEx

type GUID = efiguid.GUID

//sys	NtQuerySystemEnvironmentValueEx(name *windows.NTUnicodeString, guid *GUID, buf *byte, bufLen *uint32, attrs *uint32) (ntstatus error) = ntdll.NtQuerySystemEnvironmentValueEx
