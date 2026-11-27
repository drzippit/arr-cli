# arr-cli

A command-line tool for running a self-hosted *arr media stack from the terminal. It works on several Apps at once and gives output that both people and scripts can read.

## Language

**App**:
An application that arr-cli talks to, such as Sonarr, Radarr or SABnzbd.
_Avoid_: service, server, backend

**Media App**:
An App that tracks a Library of Titles and grabs Downloads for them, such as Sonarr or Radarr.
_Avoid_: *arr (on its own), manager

**Download Client**:
An App that fetches the Downloads a Media App grabs, or that someone adds to it by hand, such as SABnzbd.
_Avoid_: downloader, client (on its own), usenet client

**Instance**:
One running deployment of an App that arr-cli can reach, known by a name, a URL and an API key. An App can have several Instances (for example `sonarr` and `sonarr-4k`).
_Avoid_: server, host, connection

**Title**:
A Series or a Movie that a Media App Instance tracks. It is the shared term for commands that cover more than one App.
_Avoid_: media, item, show

**Series**:
A TV show tracked by a Sonarr Instance, made up of seasons and episodes.

**Movie**:
A film tracked by a Radarr Instance.

**Library**:
All the Titles one Media App Instance tracks.
_Avoid_: collection, catalogue

**Missing**:
A monitored Title whose files are not all there: a Movie that is available (by its minimum availability) but has no file, or a Series with an aired, monitored episode that has none. An unmonitored Title, or one not out yet, is never Missing.
_Avoid_: wanted, incomplete

**Selection**:
The Titles in one Instance's Library that a command changes all at once, chosen by what you asked for: a query, filters, a list of IDs, or picking them by hand.
_Avoid_: batch, bulk set, media

**Candidate**:
A Series or Movie that an App's lookup returns as something that could be added to an Instance. A Candidate may match a Title already in that Instance's Library, and adding a Candidate makes it a Title.
_Avoid_: result, match, lookup item

**Release**:
Something an indexer offers to download for a Title, known by its name, such as `Severance.S02E07.1080p.WEB-DL-EXAMPLE`. Grabbing a Release makes it a Download.
_Avoid_: result, NZB, torrent (for the offer itself)

**Hunt**:
Asking a Media App to look through its indexers for Releases of the Titles in a Selection, and grab the best ones.
_Avoid_: search (that finds Candidates), fetch, grab

**Download**:
One NZB or torrent that a Download Client is fetching or post-processing, or has finished with. The same Download can appear both in a Download Client and in the Media App that grabbed it.
_Avoid_: job, slot, item, grab

**Queue**:
The Downloads on an Instance that have not finished yet, including those still being post-processed.
_Avoid_: activity, active downloads

**History**:
The Downloads on an Instance that have finished, whether they completed or failed.
_Avoid_: log, past downloads

**Health Check**:
A problem an Instance reports about itself, such as an unreachable indexer or a missing root folder. Each has a severity of notice, warning or error.
_Avoid_: health item, alert, issue

**Warning**:
A Health Check that needs attention, meaning one whose severity is warning or error. Notices are not Warnings.
_Avoid_: health issue, problem

**App version**:
The version of its App that an Instance reports, such as Sonarr `4.0.11.2680`. arr-cli reads it but never changes it.
_Avoid_: version (on its own), server version

**arr version**:
A tagged, published build of arr-cli, such as `v0.3.0`. "arr's version" always means the arr version you are running.
_Avoid_: Release (that is what an indexer offers), build
