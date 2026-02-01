/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package uptimerobot

import (
	"net/url"
	"strings"

	uptimerobotv1 "github.com/joelp172/uptime-robot-operator/api/v1alpha1"
)

// FindOpt is a function that modifies query parameters for monitor search.
type FindOpt func(params url.Values)

// FindBySearch sets the search query parameter for filtering monitors.
func FindBySearch(val string) FindOpt {
	return func(params url.Values) {
		params.Set("search", val)
	}
}

// FindByURL creates a FindOpt that searches for monitors by URL.
func FindByURL(monitor uptimerobotv1.MonitorValues) FindOpt {
	return FindBySearch(monitor.URL)
}

// FindByID creates a FindOpt that filters monitors by ID(s).
// In v3 API, multiple IDs are comma-separated.
func FindByID(id ...string) FindOpt {
	return func(params url.Values) {
		params.Set("ids", strings.Join(id, ","))
	}
}
