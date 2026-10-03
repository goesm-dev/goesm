// Package sideeffect is imported only for its init function.
package sideeffect

import "example.com/sem/initdeps/registry"

func init() { registry.Inited = append(registry.Inited, "sideeffect") }
