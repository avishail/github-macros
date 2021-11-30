# Github Macros - Server Side

The serve side of Github Macros is running on Google Cloud Function. It provides a basic
query and mutation capabilities

## Query Options
search - search for all macros containing the provided text. Paging is supported.

get - given list of macro names, return the metadata of the existing ones.

suggestion - get macro suggestions. Paging is supported.

## Mutate Options
add - add a new macro.

use - mark a usage of the macro.

report - report that macro's URL is broken.
