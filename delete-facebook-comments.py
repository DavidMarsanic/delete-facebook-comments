import os
import sys
import subprocess
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
import time

os.environ["webdriver.chrome.driver"] = "./chromedriver"
options = webdriver.ChromeOptions()
options.add_experimental_option("debuggerAddress", "127.0.0.1:9222")
browser = webdriver.Chrome(options=options)

# Navigate directly to the activity log for comments
browser.get('https://www.facebook.com/1322687115/allactivity?activity_history=false&category_key=COMMENTSCLUSTER&manage_mode=false&should_load_landing_page=false')

def select_and_remove():
    try:
        # Attempt to click the checkbox
        checkbox = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.NAME, 'comet_activity_log_select_all_checkbox'))
        )
        if checkbox.is_displayed():
            checkbox.click()
            print("Checkbox clicked successfully!")
            
            # Wait for the "Remove" button to be clickable
            remove_button = WebDriverWait(browser, 10).until(
                EC.element_to_be_clickable((By.XPATH, "//span[text()='Remove']"))
            )
            # Click the "Remove" button
            remove_button.click()
        else:
            print("Checkbox is obscured. Trying JavaScript click.")
            browser.execute_script("arguments[0].click();", checkbox)
            print("Checkbox clicked using JavaScript!")
    except Exception as e:
        print("Error:", e)

def confirm_remove():
    try:
        # Wait for the confirmation dialog to be displayed
        confirm_dialog = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.XPATH, "//div[contains(text(),'This action cannot be undone.')]/ancestor::div[@role='dialog']"))
        )

        # If the confirmation dialog is displayed, proceed to confirm removal
        if confirm_dialog:
            # Locate the "Remove" button in the confirmation dialog and click it
            confirm_remove_button = WebDriverWait(browser, 10).until(
                EC.element_to_be_clickable((By.XPATH, "//div[@aria-label='Remove']"))
            )
            confirm_remove_button.click()
            print("Comments removed successfully!")
    except Exception as e:
        print("Error:", e)

while True:
    input("Press Enter to run the delete comments logic or Ctrl+C to exit.")
    select_and_remove()
    time.sleep(1)
    # Call the checkbox killer script
    subprocess.run([sys.executable, "/Users/david/Documents/Code/delete-facebook-comments/checkbox-killer"])

