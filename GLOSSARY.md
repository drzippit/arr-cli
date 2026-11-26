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
