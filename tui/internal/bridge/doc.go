// Package bridge serves Momentum's task and domain operations to a native
// frontend over newline-delimited JSON on stdin and stdout. It reuses the
// TUI's backend, domain, quickadd and config packages so every frontend
// shares one implementation of views, capture and edits.
package bridge
