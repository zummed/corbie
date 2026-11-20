# Pages

The rules behind [the Pages overview](../page.md). Area code: `page`.

## The header

**R-page-01** The header offers the pages the UI's parties may use, and names the parties, or says the UI is a viewer. *Test:* `page_header_offers_the_parties_pages`

## The look

**C-page-01** No page of the UI loads anything from another site: its fonts, its style, its script and Solid are in the binary. *Test:* `page_loads_nothing_from_another_site`

## Following the store

**R-page-02** A page is sent every part as it is when it connects, then each part whose data changed, and is told when the store cannot be read and when it can again. *Test:* `page_stream_sends_every_part_then_what_changed`

**C-page-02** A page follows the store over a WebSocket. A browser holds no more than six requests open to one address over HTTP/1.1, and a stream of events holds one for as long as its page is open, so a seventh page would wait. *Test:* `page_follows_over_a_websocket`

## Interfaces

Pages uses no other area of the UI.
