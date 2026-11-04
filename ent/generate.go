// Package ent is the generated ORM: the entities, their builders and the scoped client (ADR 0017)
// that entc writes from ent/schema. Nothing in it but this file is edited by hand.
package ent

//go:generate go run -mod=mod entc.go
//go:generate go run -mod=mod entc.go -schema ./schema/scopedtest -target ./scopedtest -package github.com/getsphericon/sphericon/ent/scopedtest -snapshot=false
