# Ward integration map

Use this map when a change crosses AOS, Ward, agent-compose, or AOSguard.

## Find the current surfaces

* `rg -n "context-bundle|--warded|--composed|--guarded" aos docs`
* `ward doctor`
* `aosguard ops forgejo describe`

## Notes

* `.agents/roles.kdl` owns behavioral composition.
* `.ward/ward.yaml` owns repository command declarations and supported YAML
  fixture input.
* Ward owns fixed workflows and its broker. AOSguard owns its separate static
  operator surface.
