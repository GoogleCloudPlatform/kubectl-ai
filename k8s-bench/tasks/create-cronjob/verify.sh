#!/usr/bin/env bash

# Check if the CronJob exists
if ! kubectl get cronjob data-backup -n create-cronjob-test &>/dev/null; then
  echo "CronJob 'data-backup' not found in namespace 'create-cronjob-test'"
  exit 1
fi

# Verify schedule is set to midnight
# Accept either 0 0 * * * or @daily
SCHEDULE=$(kubectl get cronjob data-backup -n create-cronjob-test -o jsonpath='{.spec.schedule}')
if [[ "$SCHEDULE" != "0 0 * * *" && "$SCHEDULE" != "@daily" ]]; then
  echo "CronJob schedule is not set to midnight (0 0 * * * or @daily). Found: $SCHEDULE"
  exit 1
fi

# Verify the container image is busybox
IMAGE=$(kubectl get cronjob data-backup -n create-cronjob-test -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].image}')
if [[ "$IMAGE" != *"busybox"* ]]; then
  echo "Container image is not busybox. Found: $IMAGE"
  exit 1
fi

# Check for the command or args containing the echo statement
# Try both command and args as the LLM might use either approach
CMD=$(kubectl get cronjob data-backup -n create-cronjob-test -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].command}' 2>/dev/null || echo "")
ARGS=$(kubectl get cronjob data-backup -n create-cronjob-test -o jsonpath='{.spec.jobTemplate.spec.template.spec.containers[0].args}' 2>/dev/null || echo "")

# Check if either command or args contain the required text
if [[ ("$CMD" == *"echo"* && "$CMD" == *"Backup completed"*) || ("$ARGS" == *"echo"* && "$ARGS" == *"Backup completed"*) ]]; then
  # Command format is correct
  echo "Command validation passed"
else
  # If not in command/args directly, check if it's in a shell command
  if [[ "$CMD" == *"/bin/sh"* || "$CMD" == *"/bin/bash"* ]]; then
    # It's a shell command, check if args contain echo
    if [[ "$ARGS" == *"echo"* && "$ARGS" == *"Backup completed"* ]]; then
      echo "Command validation passed (shell with echo in args)"
    else
      echo "Command does not include 'echo Backup completed'"
      exit 1
    fi
  elif [[ "$CMD" == "" && "$ARGS" == "" ]]; then
    # Neither command nor args are set, which is unusual
    echo "Warning: Neither command nor args are set in the CronJob"
    exit 1
  else
    echo "Command does not include 'echo Backup completed'"
    exit 1
  fi
fi

# If we reach this point, all checks passed
echo "All verification checks passed for CronJob 'data-backup'"
exit 0 