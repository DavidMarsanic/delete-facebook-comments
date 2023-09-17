import os
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.common.action_chains import ActionChains
import time

os.environ["webdriver.chrome.driver"] = "./chromedriver"
browser = webdriver.Chrome()

# Navigate to Facebook
browser.get('https://www.facebook.com/')

# Allow the user to log in manually
input("Please log in to Facebook and then press Enter here to continue...")

# Navigate to the activity log for comments
browser.get('https://www.facebook.com/1322687115/allactivity?activity_history=false&category_key=COMMENTSCLUSTER&manage_mode=false&should_load_landing_page=false')

def delete_comments():
    try:
        # Wait for the checkbox to be present
        checkbox = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.NAME, 'comet_activity_log_select_all_checkbox'))
        )

        # Scroll to the bottom of the page to ensure "Remove" button is in view
        browser.execute_script("window.scrollTo(0, document.body.scrollHeight);")

        # Use ActionChains to click the checkbox
        actions = ActionChains(browser)
        actions.move_to_element(checkbox)
        actions.click(checkbox)
        actions.perform()

        # Wait for the "Remove" button to be clickable
        remove_button = WebDriverWait(browser, 10).until(
            EC.element_to_be_clickable((By.XPATH, "//span[text()='Remove']"))
        )
        
        # Click the "Remove" button
        remove_button.click()

        # Check if the confirmation dialog is displayed
        confirm_dialog = WebDriverWait(browser, 3).until(
            EC.presence_of_element_located((By.XPATH, "//div[contains(text(),'This action cannot be undone.')]"))
        )

        # If the confirmation dialog is displayed, proceed to confirm removal
        if confirm_dialog:
            time.sleep(1)
            
            # Locate the "Remove" button in the confirmation dialog and click it
            confirm_remove_button = WebDriverWait(browser, 10).until(
                EC.element_to_be_clickable((By.XPATH, "//button[@data-testid='delete_button']"))
            )
            confirm_remove_button.click()
        
    except Exception as e:
        print("Error:", e)

while True:
    input("Press Enter to run the delete comments logic or Ctrl+C to exit.")
    delete_comments()
