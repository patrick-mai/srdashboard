// Package recovery handles session logs and replaying pasted OpticScore logs.
//
// Paste path: ParseLog (console "UDP: shot applied …" or Event/Shot JSON)
// → Listener.ReplayLog → Pipeline.IngestReplay (same validation + OnShot as live UDP).
// Game plugins therefore reconstruct from recovered shots; random field events
// are not in the log and will not match the original session.

package recovery
