package i18n

// Catalog 是一个语言的全部文案。按使用它的包分组；新增字段时两种语言都要填，
// TestCatalogsComplete 会检查空字段，避免漏翻。
type Catalog struct {
	CLI    CLIText
	Share  ShareText
	Filex  FilexText
	Auth   AuthText
	Config ConfigText
	Crypto CryptoText
	API    APIText
	Term   TermText
}

type CLIText struct {
	Usage string

	UsageLine      string
	OptionsHeader  string
	NoSuchCommand  string
	UnknownCommand string
	SeeHelpHint    string
	HelpOnlyOne    string
	SaveOnlyOne    string
	Canceled       string

	SynLogin   string
	SynLogout  string
	SynWhoami  string
	SynShare   string
	SynSave    string
	SynList    string
	SynRm      string
	SynQuota   string
	SynHelp    string
	SynVersion string

	SumLogin   string
	SumLogout  string
	SumWhoami  string
	SumShare   string
	SumSave    string
	SumList    string
	SumRm      string
	SumQuota   string
	SumHelp    string
	SumVersion string

	FlagPassword     string
	FlagMaxDownloads string
	FlagCopy         string
	FlagClipboard    string
	FlagOut          string
	FlagSavePassword string
	FlagConcurrency  string
	FlagNoBrowser    string

	LoggedIn        string
	LoggedOut       string
	NotLoggedIn     string
	CurrentAccount  string
	NeedLogin       string
	ShareNeedsLogin string

	SetPasswordPrompt    string
	RepeatPasswordPrompt string
	PasswordMismatch     string
	PasswordTooShort     string
	SavePasswordPrompt   string
	PasswordHint         string
	KeyHint              string

	CopyFailed           string
	ClipboardReadFailed  string
	MaxDownloadsNegative string
	ConcurrencyNegative  string
	Saved                string

	NoFiles    string
	ColID      string
	ColSize    string
	ColExpires string

	Deleted      string
	DeleteFailed string
	QuotaLine    string
}

type ShareText struct {
	DirectoryNotFile string
	FileTooLarge     string
	ContentTooLarge  string
	QuotaExceeded    string
	QuotaHint        string
	ReadFailed       string
	FileChanged      string
	UploadPartFailed string
	UploadPartStatus string
	MissingETag      string
	MissingPartURL   string
	ProgressLabel    string
}

type FilexText struct {
	MissingKey          string
	EmptyTarget         string
	BadURL              string
	MissingFileID       string
	UnrecognizedTarget  string
	BadKey              string
	NotFound            string
	PasswordRequired    string
	DownloadUnavailable string
	SizeMismatch        string
	ChunkDownloadFailed string
	ChunkVerifyFailed   string
	StdoutConcurrent    string
	HTTPStatus          string
	TransferFailed      string
	RangeIgnored        string
	ProgressLabel       string
}

type AuthText struct {
	OpenLink        string
	DeviceCode      string
	BrowserFallback string
	BrowserOpened   string
	NoToken         string
	Denied          string
	Expired         string
}

type ConfigText struct {
	NoConfigDir string
	Corrupt     string
}

type CryptoText struct {
	WrongPassword    string
	BadFileID        string
	MasterKeyLength  string
	ChunkTooLarge    string
	ChunkNotFull     string
	ChunkTooShort    string
	ChunkAuthFailed  string
	MetaTooLong      string
	MetaBadBase64    string
	MetaTooShort     string
	MetaAuthFailed   string
	MetaBadJSON      string
	MetaVersion      string
	BadName          string
	BadSize          string
	BadType          string
	BadChunkSize     string
	BadChunkCount    string
	PasswordTooShort string
	BadWrapSalt      string
	BadWrapCipher    string
	BadWrapLength    string
	UnsupportedKDF   string
	WeakKDF          string
}

type APIText struct {
	ConnectFailed string
	DecodeFailed  string
}

type TermText struct {
	EmptyInput      string
	Expired         string
	LessThanMinute  string
	MinutesLeft     string
	HoursLeft       string
	DaysLeft        string
	NoClipboardTool string
	ClipboardEmpty  string
}
