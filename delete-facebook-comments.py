import os
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

def click_element_using_js(element):
    browser.execute_script("arguments[0].click();", element)

def delete_comments():
    try:
        # Attempt to click the checkbox
        checkbox = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.NAME, 'comet_activity_log_select_all_checkbox'))
        )
        if checkbox.is_displayed():
            checkbox.click()
        else:
            print("Checkbox is obscured. Skipping to next step.")
    except Exception as e:
        print("Checkbox click intercepted. Skipping to next step.")

    # Use JavaScript to force click the "Remove" button in the confirmation box
    try:
        remove_button = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.XPATH, "//div[contains(text(),'Remove')]"))
        )
        click_element_using_js(remove_button)
    except Exception as e:
        print("Error:", e)

while True:
    input("Press Enter to run the delete comments logic or Ctrl+C to exit.")
    delete_comments()
