import os
import sys
import subprocess
import time
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

os.environ["webdriver.chrome.driver"] = "./chromedriver"
options = webdriver.ChromeOptions()
options.add_experimental_option("debuggerAddress", "127.0.0.1:9222")
browser = webdriver.Chrome(options=options)

# Navigate directly to the activity log for comments
browser.get('https://web.facebook.com/1322687115/allactivity?activity_history=false&category_key=MANAGEPOSTSPHOTOSANDVIDEOS&manage_mode=false&should_load_landing_page=false')

def select_and_remove():
    try:
        # Attempt to click the checkbox
        checkbox = browser.find_element(By.NAME, 'comet_activity_log_select_all_checkbox')
        if checkbox.is_displayed():
            checkbox.click()
            print("Checkbox clicked successfully!")
            
            # Click the "Remove" button
            remove_button = browser.find_element(By.XPATH, "//span[text()='Trash']")
            remove_button.click()
    except Exception as e:
        print("Error:", e)

def final_confirmation():
    try:
        confirm_button = browser.find_element(By.XPATH, "//div[@aria-label='Move to trash'][@role='button']")
        confirm_button.click()
    except Exception as e:
        print("Error during final confirmation:", e)

def archive_button():
    try:
        archive_btn = browser.find_element(By.XPATH, "//div[@aria-label='Move to archive'][@role='button']")
        archive_btn.click()
    except Exception as e:
        print("Error during archiving:", e)

def archive_confirmation():
    try:
        confirm_archive_btn = browser.find_element(By.XPATH, "//div[@aria-label='Move to archive'][@role='button']")
        confirm_archive_btn.click()
    except Exception as e:
        print("Error during archive confirmation:", e)

while True:
    select_and_remove()
    time.sleep(1)
    # Call the checkbox killer script
    subprocess.run([sys.executable, "/Users/david/Documents/Code/delete-facebook-comments/checkbox-killer"])
    final_confirmation()
    
    # Continuously check if the first checkbox has become clickable every second
    while True:
        try:
            checkbox = WebDriverWait(browser, 1).until(EC.element_to_be_clickable((By.NAME, 'comet_activity_log_select_all_checkbox')))
            if checkbox:
                print("Checkbox is clickable again. Proceeding with the next deletion cycle.")
                break
        except:
            print("Waiting for checkbox to become clickable...")
            time.sleep(1)

# You can continue with other operations or close the browser if needed.
# browser.quit()
