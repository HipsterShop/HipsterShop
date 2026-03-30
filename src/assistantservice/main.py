import os
import requests
from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel
from langchain_google_genai import ChatGoogleGenerativeAI
from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.prebuilt import create_react_agent
from langchain_core.tools import tool
from contextvars import ContextVar

GATEWAY_ADDR = os.getenv("GATEWAY_ADDR", "hipstershop-gateway.hipster.svc.cluster.local:80")
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")

app = FastAPI()

class ChatRequest(BaseModel):
    productId: str
    message: str

current_session_id = ContextVar("current_session_id", default="assistant_bot_session")

@tool
def get_product_details(product_id: str) -> str:
    """Fetch exhaustive details about a specific product in the store including price, description, and specifications."""
    try:
        r = requests.get(f"http://{GATEWAY_ADDR}/api/products/{product_id}", timeout=5)
        if r.status_code == 200:
            return r.text
        return f"Product not found. Status: {r.status_code}"
    except Exception as e:
        return f"Error fetching product: {e}"

@tool
def add_to_cart(product_id: str, quantity: int) -> str:
    """Add a specified quantity of a product to the user's shopping cart. Use this ONLY if the user explicitly asks to add it to their cart or buy it."""
    session_id = current_session_id.get()
    payload = {
        "userId": session_id,
        "item": {
            "productId": product_id,
            "quantity": quantity
        }
    }
    try:
        r = requests.post(f"http://{GATEWAY_ADDR}/api/cart/add", json=payload, timeout=5)
        if r.status_code < 400:
            return f"Successfully added {quantity} of {product_id} to the cart!"
        return f"Failed to add to cart. Status: {r.status_code}"
    except Exception as e:
        return f"Error adding to cart: {e}"

tools = [get_product_details, add_to_cart]

system_instruction = """You are a highly capable agentic shopping assistant for HipsterShop.
Your goals:
1. Answer customer questions regarding products by using the 'get_product_details' tool to fetch the required information.
2. If the user explicitly asks to "add it to my cart", "buy it", or "put 2 of these in my cart", use the 'add_to_cart' tool to perform that action for them!
3. After performing an action (like adding to cart), verbally confirm to the user that you did it and ask if they need anything else.
Keep your final answers concise, conversational, and completely devoid of Markdown formatting (no asterisks, no bolding, no bullets) as the UI is simple text only.
"""

@app.post("/api/assistant/chat")
def chat_endpoint(req: ChatRequest, request: Request):
    if not GEMINI_API_KEY:
        raise HTTPException(status_code=503, detail="Assistant API key missing")

    session_id = "guest_session"
    if "shop_session-id" in request.cookies:
        session_id = request.cookies["shop_session-id"]

    current_session_id.set(session_id)

    user_msg = req.message
    if not user_msg:
        user_msg = "Give me a quick summary of this product and an engaging reason to buy it."

    prompt = f"The user is currently viewing Product ID: {req.productId}.\n\nUser Question: {user_msg}"

    llm = ChatGoogleGenerativeAI(model="gemini-2.5-flash", google_api_key=GEMINI_API_KEY, temperature=0.2)
    agent_executor = create_react_agent(llm, tools)

    messages = [
        SystemMessage(content=system_instruction),
        HumanMessage(content=prompt)
    ]

    try:
        response = agent_executor.invoke({"messages": messages})
        final_message = response["messages"][-1].content
        
        final_message = final_message.replace("**", "").replace("`", "").replace("_", "")
        return {"reply": final_message}
    except Exception as e:
        print(f"Agent error: {e}")
        raise HTTPException(status_code=502, detail="Agent request failed")

@app.get("/_healthz")
def healthz():
    return "ok"

@app.options("/api/assistant/chat")
def options_chat():
    return "ok"
