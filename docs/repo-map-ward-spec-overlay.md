# Ward integration map

Use this map when a change crosses AOS, Ward, agent-compose, or AOSguard.

## Find the current surfaces

* `rg -n "context-bundle|--warded|--composed|--guarded" aos docs`
* `ward doctor`
* `aosguard ops forgejo describe`

## Notes

* `.agents/roles.kdl` owns behavioral composition.
* `aos/harness_launch_profiles.json` owns concrete harness launch tuning.
* `.ward/ward.yaml` owns repository commands, the deployment image and release
  channel, and supported YAML fixture input.
* Agent-compose owns named seats and pronouns.
* Ward owns fixed workflows and its broker. AOSguard owns its separate static
  operator surface.
