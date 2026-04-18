#!/usr/bin/env bash
# Small synthetic shell script for unit tests.

# greet prints a greeting for the given name.
greet() {
    local name="$1"
    echo "Hello, ${name}!"
}

# add prints the sum of two numbers.
add() {
    local a="$1"
    local b="$2"
    echo $(( a + b ))
}

greet "World"
add 3 4
