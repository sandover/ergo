# Ergo Backlog icon source

The Marketplace icon is a lowercase `e` set in Inconsolata Medium at its
widest variable-font width. The warm glyph fills the square with a narrow
optical margin over a restrained charcoal-to-deep-teal gradient.

## Provenance

- Typeface: Inconsolata by Raph Levien and contributors
- Typeface license: SIL Open Font License 1.1
- Source: <https://github.com/googlefonts/Inconsolata>
- Glyph: `#f2e8d5`
- Gradient: `#111820`, `#14262d`, `#12343a`

The font file is an export dependency and is not included in the extension.

## Export

Install Inconsolata, then render from the extension directory:

```sh
rsvg-convert --width 256 --height 256 \
  assets/source/icon.svg \
  --output assets/icon.png
```

Inspect the PNG at 16, 32, 64, 128, and 256 pixels on light and dark
surroundings before publication.
