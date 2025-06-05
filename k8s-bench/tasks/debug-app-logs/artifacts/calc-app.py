import sys
import time

print("Starting calc-app...")
sys.stdout.flush()

counter = 0
while True:
    counter += 1
    if counter % 4 == 0:
        try:
            result = 1 / 0
        except ZeroDivisionError as e:
            print(f"Run {counter} failed with error: {e}")
            sys.stdout.flush()
    else:
        print(f"Run {counter} succeeded")
        sys.stdout.flush()
    
    time.sleep(1)
