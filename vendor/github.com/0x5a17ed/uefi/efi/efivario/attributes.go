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

package efivario

//go:generate go run github.com/hexaflex/stringer -flags -type=Attributes attributes.go

// Attributes indicates how the data variable should be stored and
// maintained by the system.
//
// The attributes affect when the variable may be accessed and
// volatility of the data.
//
// <https://uefi.org/sites/default/files/resources/UEFI_Spec_2_9_2021_03_18.pdf#I12.1.1356525>
type Attributes uint32

const (
	// NonVolatile indicates the firmware environment variable is
	// stored in non-volatile memory (e.g. NVRAM).
	NonVolatile Attributes = 0x0001

	// BootServiceAccess indicates whenever the variable is visible
	// for boot services.
	BootServiceAccess Attributes = 0x0002

	// RuntimeAccess indicates whenever the variable is visible
	// after ExitBootServices() in UEFI is called.
	RuntimeAccess Attributes = 0x0004

	// HardwareErrorRecord marks the variable to be stored in an
	// area for hardware errors.
	HardwareErrorRecord Attributes = 0x0008

	// AuthenticatedWriteAccess is deprecated and should no longer
	// be used.
	AuthenticatedWriteAccess Attributes = 0x0010

	TimeBasedAuthenticatedWriteAccess Attributes = 0x0020

	AppendWrite Attributes = 0x0040

	// EnhancedAuthenticatedAccess attribute indicates that the
	// variable payload begins with an
	// EFI_VARIABLE_AUTHENTICATION_3 structure, and potentially
	// more structures as indicated by fields of this structure.
	EnhancedAuthenticatedAccess Attributes = 0x0080
)
