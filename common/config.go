package common

// MatchflowConfig configuration for matchflow auditor
type MatchflowConfig struct {
	FullEpoch    string
	PartialEpoch string
	InitialAudit bool
	DbAddress    string
	DbPass       string
	Pausable     bool
	DexStartTime string
	DbUser       string
}
