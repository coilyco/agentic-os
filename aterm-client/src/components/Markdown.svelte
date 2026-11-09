<script lang="ts">
  import { parseBlocks, type Block } from "../lib/markdown";
  import InlineText from "./InlineText.svelte";

  let { source, onlink }: { source: string; onlink?: (href: string) => void } = $props();
  const blocks = $derived(parseBlocks(source));
</script>

{#snippet render(list: Block[])}
  {#each list as block, at (at)}
    {#if block.t === "p"}<p><InlineText nodes={block.c} {onlink} /></p>
    {:else if block.t === "h"}<p class="h h{block.level}" role="heading" aria-level={block.level}><InlineText nodes={block.c} {onlink} /></p>
    {:else if block.t === "code"}<pre><code>{block.v}</code></pre>
    {:else if block.t === "quote"}<blockquote>{@render render(block.c)}</blockquote>
    {:else if block.t === "hr"}<hr />
    {:else if block.t === "list"}
      {#if block.ordered}
        <ol start={block.start}>{#each block.items as item, row (row)}<li>{@render render(item)}</li>{/each}</ol>
      {:else}
        <ul>{#each block.items as item, row (row)}<li>{@render render(item)}</li>{/each}</ul>
      {/if}
    {:else if block.t === "table"}
      <!-- A wide table scrolls by keyboard. -->
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <div class="wide" tabindex="0" role="region" aria-label="Table">
        <table>
          <thead><tr>{#each block.head as cell, col (col)}<th><InlineText nodes={cell} {onlink} /></th>{/each}</tr></thead>
          <tbody>{#each block.rows as row, r (r)}<tr>{#each row as cell, col (col)}<td><InlineText nodes={cell} {onlink} /></td>{/each}</tr>{/each}</tbody>
        </table>
      </div>
    {/if}
  {/each}
{/snippet}

<div class="md">{@render render(blocks)}</div>

<style>
  .md { font: 14px/1.5 var(--font-body); overflow-wrap: anywhere; }
  .md > :global(:first-child) { margin-top: 0; }
  .md > :global(:last-child) { margin-bottom: 0; }
  p { margin: 0 0 0.7em; white-space: pre-line; }
  .h { font-family: var(--font-display); font-weight: 600; line-height: 1.3; margin: 1em 0 0.4em; white-space: normal; }
  .h1 { font-size: 18px; }
  .h2 { font-size: 16px; }
  .h3, .h4, .h5, .h6 { font-size: 14px; }
  ul, ol { margin: 0 0 0.7em; padding-left: 1.4em; }
  li > :global(p) { margin-bottom: 0.3em; }
  li :global(ul), li :global(ol) { margin-bottom: 0.3em; }
  blockquote { margin: 0 0 0.7em; padding: 0 0 0 12px; border-left: 3px solid var(--line); color: var(--text-soft); }
  pre { margin: 0 0 0.7em; padding: 10px 12px; overflow-x: auto; border-radius: 8px; border: 1px solid var(--line); background: var(--surface); font: 13px/1.45 var(--font-mono); }
  pre code { padding: 0; border: 0; background: none; font: inherit; overflow-wrap: normal; }
  hr { border: 0; border-top: 1px solid var(--line); margin: 1em 0; }
  .wide { overflow-x: auto; margin: 0 0 0.7em; }
  .wide:focus-visible { outline: 2px solid var(--brand); outline-offset: 2px; }
  table { border-collapse: collapse; font-size: 13px; }
  th, td { padding: 4px 10px; border: 1px solid var(--line); text-align: left; vertical-align: top; }
  th { background: var(--surface); font-weight: 600; }
</style>
