package core

// FileDeliverySourceProfile names the source-cache profile that materializes a
// source copy as its plain constituent files.
//
// The literal value is persisted in source_cache_entries.profile on existing
// installations, so it must keep the string the retired device protocol
// originally defined even though MGA no longer delivers to a device agent.
// Versioned content delivery for external frontends reuses this same profile.
const FileDeliverySourceProfile = "device.files.v1"
