NAME:
   photodb - Photo library management tool

USAGE:
   photodb [global options] command [command options]

COMMANDS:
   scan       Scan photo library and save to database
   find-md5   Find photo by MD5 hash
   find-date  Find photos by date (YYYY-MM-DD)
   help, h    Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --db value  Path to SQLite database (default: "photos.db") [$PHOTODB_DATABASE]
   --help, -h  show help
