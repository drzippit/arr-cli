# Instance-first command grammar

Commands take the form `arr <instance> <command> [args]`, where `<instance>` is a name from the config (for example `arr radarr search "good will hunting"`). Leaving out the Instance (`arr status`, `arr queue`) runs a read command across every Instance. We chose this over `--instance` / `--app` flags because it reads naturally and makes every write name its target, so there is no "default Instance" to get wrong. The cost is that Instance names share a namespace with top-level commands, so a config that names an Instance after a command (such as `status`) is rejected when it loads.

`search` is the exception: it is a read command but runs only as `arr <instance> search`. A search is how you pick a Candidate to add to one Instance, and Sonarr and Radarr Candidates do not belong in one list, so there is no `arr search` across every Instance. The name is still reserved so no Instance can take it.
