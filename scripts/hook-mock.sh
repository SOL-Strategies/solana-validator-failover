#!/bin/bash

# print args received
echo "hook says hi $@"
echo "oh noes an error" 1>&2
sleep 1
echo "hook done"