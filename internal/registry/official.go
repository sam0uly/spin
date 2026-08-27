package registry

// OfficialAlias is the alias the built-in official registry is
// registered under on first run.
const OfficialAlias = "official"

// DefaultRegistryURL is the git source of the official registry that
// is bootstrapped on first run. It is a variable so tests can point
// it at a local fixture.
var DefaultRegistryURL = "https://github.com/spin-templates/registry"
