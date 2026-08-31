package tools

// Ensure zoneinfo lookups like time.LoadLocation("America/Los_Angeles")
// succeed in test binaries even on hosts without /usr/share/zoneinfo (Alpine
// containers, minimal CI images). Production binaries load zoneinfo from the
// host; this import only affects test builds.
import _ "time/tzdata"
