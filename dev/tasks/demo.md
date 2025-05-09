# steps to produce the demo gif

1. Use [asciinema](asciinema.org) to record a screencast.

2. Use agg to produce the gif

```shell
agg .github/kubectl-ai.cast --speed 1.3 --idle-time-limit 1 --theme monokai --font-size 24 --cols 100 --rows 25 .github/kubectl-ai.gif
```