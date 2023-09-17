import os
from selenium import webdriver

os.environ["webdriver.chrome.driver"] = "./chromedriver"
browser = webdriver.Chrome()
browser.get("https://www.google.com/")
browser.quit()
