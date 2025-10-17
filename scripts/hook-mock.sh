#!/bin/bash

# print args received
echo "hook says hi $@"
echo "oh noes an error" 1>&2
echo "hook done"