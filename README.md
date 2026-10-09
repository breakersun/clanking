# What's that *Clanking* sound?

For machines called "clankers", LLM agents sure don't make much noise.
Are they *too* quiet? Yes! But not anymore.

Clanking is a utility that creates a subtle soundscape from your LLM
agent activity to unobtrusively signal that the agent is working, or
waiting for input, or in one of several other states. Clanking plays
cues from many agents at once, and harmonizes them into a happy chorus
of chattering clankers.

## Design

Within your coding agent, hooks are configured to invoke the Clanker
CLI, and Clanker reads the hook event, and applies a sound cue. For
tool uses, it makes a short tick sound, at one of several notes, to
signal to the operator that the agent is active. There are also
specific sounds to indicate that the agent is waiting for input, is
done, or has encountered an error, or needs a permission prompt.

Clanking runs as a daemon, and sounds are cued by invoking the command
line app in coding harness hooks. DIfferent Sounds are quantized to
16th notes at the configured tempo, defaulting to 120 BPM. With
high activity, Clanker will scale to 32nd notes. Sounds are
synthesized within the program. 

Clanking uses the stereo field to help you distinguish one agent
session from another. Each agent is panned in the stereo field as a
function of the agent's working directory, so that clanks from
different agents will seem to come from a different location.

The tempo, BPM, and root note can be configured from the command line
(see `clanker help`) or from Clanker's GTK-4 UI.

## Installation

Clanking requires a Unix-like system with PipeWire and the `pw-cat`
utility. It also has a GTK 4 user interface for configuration.
Clanking is vibe-coded in Go, and requires a Go toolchain and GTK 4
development libraries.

On a Unix-like system with the required dependencies and a XDG-like
desktop (Gnone, KDE, etc.), `make install` will install `clanker`,
`clanker-ui`, a desktop file, and an icon into the home directory.

There's also an included `Containerfile`. That can be used to build
Clanker without installing all the development dependencies on system.
The `podman-make` script builds and invokes the container, and
`podman-make install` can perform the installation. Adapting it for
Docker is left as an exercise for the reader or the reader's coding
agent.

## Configure your Coding Agents

With the `clanking` and `pw-cat` executables in your `$PATH` on your
PipeWire-supporting system, you can configure your coding agent to
invoke Clanking.

For Claude, install the clanking plugin from this repo's marketplace:

```
/plugin install clanking --marketplace chrchr/clanking
```

For Codex, add this repository as a plugin marketplace:

```
codex plugin marketplace add chrchr/clanking
```

Restart codex, and then run `/hooks` and trust the Clanking hooks.

For Opencode, 

## Inspo

* [Peep (The Network Auralizer): Monitoring Your Network With Sound](https://www.usenix.org/legacy/publications/library/proceedings/lisa2000/full_papers/gilfix/gilfix_html/index.html) by Michael Gilfix & Prof. Alva Couch - Tufts University.

* [Star Trek: The Next Generation](https://youtu.be/Drr6_zikuZQ?si=BIBxn--yIwatbIYS). The
voice-interactive computer in the "Star Trek" franchise issues non-linguistic sounds that signal to the user, and the audience, that the computer is doing something.

* [Earcons (wikipedia)](https://en.wikipedia.org/wiki/Earcon). "An earcon is a brief, distinctive sound that represents a specific event or conveys other information."
