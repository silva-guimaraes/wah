#!/bin/bash

# Define the directory to delete files from
DIRECTORY="store/"

# Define the SQLite database and query to get the list of filenames
DATABASE="../test.db"
QUERY="SELECT hash FROM files;"

# Get the list of filenames from the database using sqlite3
IFS=$'\n' read -d '' -r -a FILENAMES < <(sqlite3 $DATABASE "$QUERY")

# Loop through all files in the directory
for FILE in $DIRECTORY/*
do
  # Check if the file is not in the list of filenames
  if [[ ! "${FILENAMES[*]}" =~ $(basename $FILE) ]]; then
    # Delete the file
    rm $FILE
  fi
done
