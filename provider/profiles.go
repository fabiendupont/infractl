// Copyright 2025 Fabien Dupont
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package provider

import "os"

const (
	// ProfileEnvVar is the environment variable that selects the deployment profile.
	ProfileEnvVar = "INFRACTL_PROFILE"

	// ProfileDefault is the default profile if none is specified.
	ProfileDefault = "default"
)

// ProfileProviders maps profile names to the provider factory functions
// that should be registered for that profile. The factory functions
// return Provider instances. This is populated by main.go or init
// functions in provider implementation packages.
var ProfileProviders = map[string][]func() Provider{}

// RegisterProfileProviders adds provider factory functions for a named
// profile. This avoids import cycles between the provider package and
// provider implementation packages.
func RegisterProfileProviders(profile string, factories []func() Provider) {
	ProfileProviders[profile] = factories
}

// GetProfile returns the active deployment profile from the INFRACTL_PROFILE
// environment variable, falling back to ProfileDefault.
func GetProfile() string {
	profile := os.Getenv(ProfileEnvVar)
	if profile == "" {
		return ProfileDefault
	}
	return profile
}

// GetProfileProviders returns the provider factories for the given profile.
// Returns nil if the profile is not found.
func GetProfileProviders(profile string) []func() Provider {
	return ProfileProviders[profile]
}
