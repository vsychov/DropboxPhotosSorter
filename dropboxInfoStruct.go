package main

//more info about struct read here - https://www.dropbox.com/help/desktop-web/find-folder-paths
type DropboxInfo struct {
	Personal DropboxInfoEntity `json:"personal"`
	Business DropboxInfoEntity `json:"business"`
}

type DropboxInfoEntity struct {
	Path             string `json:"path"`
	Host             int64  `json:"host"`
	IsTeam           bool   `json:"is_team"`
	SubscriptionType string `json:"subscription_type"`
}