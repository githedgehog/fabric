// Package itkit provides short, dead simple and concise type-safe
// generic iterator interfaces for Go.  With a few extras similar to
// what Python has to offer.
//
// # Piece of advice
//
// Abandon Hope All Ye Who Enter Here.
//
// This Go Module is probably a collection of anti-patterns. It started
// as an experiment about what can be done with the new Go generics
// functionality that was introduced into go in the 1.18 version, although
// I personally like how this module turned out to be.
//
// My recommendation for now would be to use it sparingly and use normal
// Go iteration with the for keyword where possible and only use itkit
// iterators where it makes sense to reduce code and complexity without
// sacrificing readability.
package itkit
