<script lang="ts">
  import type { Inline } from "../lib/markdown";

  // `onlink` set makes a link tappable. Unset, a link reads as a link and nothing more,
  // which is what a line inside a button or a tooltip needs.
  let { nodes, onlink }: { nodes: Inline[]; onlink?: (href: string) => void } = $props();
</script>

{#snippet render(list: Inline[])}
  {#each list as node, at (at)}
    {#if node.t === "text"}{node.v}
    {:else if node.t === "code"}<code>{node.v}</code>
    {:else if node.t === "strong"}<strong>{@render render(node.c)}</strong>
    {:else if node.t === "em"}<em>{@render render(node.c)}</em>
    {:else if node.t === "del"}<del>{@render render(node.c)}</del>
    {:else if node.t === "link" && onlink}<a href={node.href} rel="noopener noreferrer" onclick={(event) => { if (!node.href.startsWith("mailto:")) { event.preventDefault(); onlink(node.href); } }}>{@render render(node.c)}</a>
    {:else if node.t === "link"}<span class="link">{@render render(node.c)}</span>{/if}
  {/each}
{/snippet}

{@render render(nodes)}

<style>
  code { padding: 0.05em 0.3em; border-radius: 4px; background: var(--surface); border: 1px solid var(--line); font: 0.92em var(--font-mono); overflow-wrap: anywhere; }
  a, .link { color: var(--brand); text-decoration: underline; text-underline-offset: 2px; overflow-wrap: anywhere; }
  a:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
</style>
