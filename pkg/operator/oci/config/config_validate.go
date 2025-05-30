/*
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

package config

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// validateAuthConfig provides basic validation of AuthConfig instances.
func validateAuthConfig(c *AuthConfig, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if c == nil {
		return append(allErrs, field.Required(fldPath, ""))
	}
	checkFields := map[string]string{
		"region":      c.Region,
		"tenancy":     c.TenancyID,
		"user":        c.UserID,
		"key":         c.PrivateKey,
		"fingerprint": c.Fingerprint,
	}
	for fieldName, fieldValue := range checkFields {
		if fieldValue == "" {
			if fieldName == "region" {
				allErrs = append(allErrs, field.InternalError(fldPath.Child(fieldName), errors.New("This value is normally discovered automatically if omitted. Continue checking the logs to see if something else is wrong")))
			} else {
				allErrs = append(allErrs, field.Required(fldPath.Child(fieldName), ""))
			}
		}
	}
	return allErrs
}

// ValidateConfig validates the OCI Cloud Provider config file.
func ValidateConfig(c *Config) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(c.CompartmentID) == 0 {
		allErrs = append(allErrs, field.InternalError(field.NewPath("compartment"), errors.New("This value is normally discovered automatically if omitted. Continue checking the logs to see if something else is wrong")))
	}
	if !c.UseInstancePrincipals {
		allErrs = append(allErrs, validateAuthConfig(&c.Auth, field.NewPath("auth"))...)
	}
	return allErrs
}
