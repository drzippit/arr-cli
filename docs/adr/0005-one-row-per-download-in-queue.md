# One row per Download in `arr queue`

When arr-cli talks to both Media Apps and Download Clients, the same Download appears twice: once in the Media App's Queue, which knows the Title and episode it is for, and once in the Download Client's Queue, which knows its priority, category, position and live progress. `arr queue` across every Instance joins the two on the Download Client's ID (SABnzbd's `nzo_id`, the Media App's `downloadId`) and shows one row per Download. A Download that no Media App grabbed, such as an NZB added by hand, shows under its own name with no Title.

We chose this over listing both Queues side by side, which double-counts every grabbed Download, and over a separate `arr downloads` command, which would leave "what is downloading?" split across two commands. The cost is that a row in `arr queue --json` no longer always comes from one Instance: a joined row names both the Media App Instance and the Download Client Instance, and a script that counted rows per Instance must account for that. The new fields are additions, so under ADR 0004 this is not a breaking change.

`arr <instance> queue` is not joined. It shows only what that one Instance reports, as ADR 0002 promises for a single named Instance.
