import os
import sys
import subprocess
import time
from selenium import webdriver
from selenium.webdriver.common.by import By
import random

os.environ["webdriver.chrome.driver"] = "./chromedriver"
options = webdriver.ChromeOptions()
options.add_experimental_option("debuggerAddress", "127.0.0.1:9222")
browser = webdriver.Chrome(options=options)

# Navigate directly to the activity log for comments
browser.get('https://www.facebook.com/1322687115/allactivity?activity_history=false&category_key=COMMENTSCLUSTER&manage_mode=false&should_load_landing_page=false')

def select_and_remove():
    try:
        # Attempt to click the checkbox
        checkbox = browser.find_element(By.NAME, 'comet_activity_log_select_all_checkbox')
        if checkbox.is_displayed():
            checkbox.click()
            print("Checkbox clicked successfully!")
            
            # Click the "Remove" button
            remove_button = browser.find_element(By.XPATH, "//span[text()='Remove']")
            remove_button.click()
    except Exception as e:
        print("Error:", e)

def confirm_remove():
    try:
        # Wait for the confirmation dialog to be displayed
        confirm_dialog = browser.find_element(By.XPATH, "//div[contains(text(),'This action cannot be undone.')]/ancestor::div[@role='dialog']")
        
        # If the confirmation dialog is displayed, proceed to confirm removal
        if confirm_dialog:
            # Locate the "Remove" button in the confirmation dialog and click it
            confirm_remove_button = browser.find_element(By.XPATH, "//div[@aria-label='Remove']")
            confirm_remove_button.click()
            print("Comments removed successfully!")
    except Exception as e:
        print("Error:", e)

def final_confirmation():
    try:
        confirm_button = browser.find_element(By.CSS_SELECTOR, 'div[aria-label="Remove"][role="button"] > div:not([style])')
        confirm_button.click()
    except Exception as e:
        print("Error during final confirmation:", e)

while True:
    input("Press Enter to run the delete comments logic or Ctrl+C to exit.")
    select_and_remove()
    time.sleep(1)
    # Call the checkbox killer script
    subprocess.run([sys.executable, "/Users/david/Documents/Code/delete-facebook-comments/checkbox-killer"])
    final_confirmation()
    
    # Wait for a random interval between 10 to 15 seconds before rerunning the process
    sleep_time = random.randint(10, 15)
    print(f"Waiting for {sleep_time} seconds before rerunning...")
    time.sleep(sleep_time)

# You can continue with other operations or close the browser if needed.
# browser.quit()
