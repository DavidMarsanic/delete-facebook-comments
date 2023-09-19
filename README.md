Facebook Comment Deletion Script
This script automates the process of deleting comments from your Facebook activity log. It uses Selenium with Python to interact with the Facebook interface and remove comments.

Prerequisites
Python 3.x
Selenium: Install using pip install selenium
Google Chrome
ChromeDriver:
The repository includes a ChromeDriver binary compatible with macOS. If you're using a different operating system, you'll need to download the appropriate ChromeDriver for your system from the official site and ensure it's in the same directory as the script or in your PATH.
How to Use
Step 1: Start a Custom Chrome Instance
Before running the script, you need to start a custom instance of Google Chrome. This won't interfere with your existing Chrome sessions.

Run the following command in your terminal:

bash
Copy code
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome --remote-debugging-port=9222 --user-data-dir="/tmp/chrome_temp"
This command starts a new Chrome instance that the script can attach to without affecting your main Chrome profile.

Step 2: Run the Script
Navigate to the directory containing the script and run:

bash
Copy code
python script_name.py
Replace script_name.py with the name you've given to the script.

Step 3: Let the Script Work
Once the script starts, it will navigate to your Facebook activity log for comments and begin the deletion process. The script will continuously check for comments and delete them. If there are no comments left or if there's a delay in loading, the script will wait until the checkbox to select comments becomes clickable again and then continue the deletion process.

Notes

Facebook will occasionally prompt you to write your facebook password. Just input it and let the program run. 
Ensure that the ChromeDriver version matches the version of Chrome you have installed.
The script uses a specific Chrome instance and won't interfere with your regular Chrome sessions.
