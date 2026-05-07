import streamlit as st
from sklearn.datasets import load_iris
from sklearn.tree import DecisionTreeClassifier

iris = load_iris()
model = DecisionTreeClassifier(random_state=42)
model.fit(iris.data, iris.target)

st.title("Iris Species Predictor")
features = [
    [
        st.slider("Sepal length", 4.0, 8.0, 5.8),
        st.slider("Sepal width", 2.0, 4.5, 3.0),
        st.slider("Petal length", 1.0, 7.0, 4.0),
        st.slider("Petal width", 0.1, 2.5, 1.2),
    ]
]
st.subheader(f"Predicted: {iris.target_names[model.predict(features)[0]]}")
