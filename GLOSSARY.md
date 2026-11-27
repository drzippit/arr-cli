# arr-cli

A command-line tool for running a self-hosted *arr media stack from the terminal. It works on several Apps at once and gives output that both people and scripts can read.

## Language

**App**:
One of the *arr applications that arr-cli talks to, such as Sonarr or Radarr.
_Avoid_: service, server, backend

**Instance**:
One running deployment of an App that arr-cli can reach, known by a name, a URL and an API key. An App can have several Instances (for example `sonarr` and `sonarr-4k`).
_Avoid_: server, host, connection

**Title**:
A Series or a Movie that an Instance tracks. It is the shared term for commands that cover more than one App.
_Avoid_: media, item, show

**Series**:
A TV show tracked by a Sonarr Instance, made up of seasons and episodes.

**Movie**:
A film tracked by a Radarr Instance.

**Library**:
All the Titles one Instance tracks.
_Avoid_: collection, catalogue

**Candidate**:
A Series or Movie that an App's lookup returns as something that could be added to an Instance. A Candidate may match a Title already in that Instance's Library, and adding a Candidate makes it a Title.
_Avoid_: result, match, lookup item

**Health Check**:
A problem an Instance reports about itself, such as an unreachable indexer or a missing root folder. Each has a severity of notice, warning or error.
_Avoid_: health item, alert, issue

**Warning**:
A Health Check that needs attention, meaning one whose severity is warning or error. Notices are not Warnings.
_Avoid_: health issue, problem

**App version**:
The version of its App that an Instance reports, such as Sonarr `4.0.11.2680`. arr-cli reads it but never changes it.
_Avoid_: version (on its own), server version

**Release**:
A tagged, published build of arr-cli, such as `v0.2.0`. "arr's version" always means the Release you are running.
_Avoid_: version (on its own), build
