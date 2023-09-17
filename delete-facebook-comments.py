import os
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

os.environ["webdriver.chrome.driver"] = "./chromedriver"
browser = webdriver.Chrome()

# Navigate to Facebook
browser.get('https://www.facebook.com/')

# Allow user to log in manually
input("Please log in to Facebook and then press Enter here to continue...")

# Navigate to the activity log for comments
browser.get('https://www.facebook.com/1322687115/allactivity?activity_history=false&category_key=COMMENTSCLUSTER&manage_mode=false&should_load_landing_page=false')

def delete_comments():
    try:
        # Locate the checkbox and select it
        checkbox = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.NAME, 'comet_activity_log_select_all_checkbox'))
        )
        checkbox.click()

        # Locate the "Remove" button and click it
        remove_button = WebDriverWait(browser, 10).until(
            EC.presence_of_element_located((By.XPATH, "//span[text()='Remove']"))
        )
        remove_button.click()
        
    except Exception as e:
        print("Error:", e)

while True:
    input("Press Enter to run the delete comments logic or Ctrl+C to exit.")
    delete_comments()
